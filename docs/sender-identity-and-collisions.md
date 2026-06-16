# Sender identity & device collisions (Sparkplug B)

How goGateway decides *who sent a message*, where that breaks down when many
devices (or a malicious/misconfigured one) are on the bus, and what to do about
it. Validated with `scripts/soak-edgecases.sh`.

> Related: [`sparkplug-contract.md`](sparkplug-contract.md),
> [`e2e-test-guide.md`](e2e-test-guide.md).

---

## 1. How identity actually works

goGateway is a Sparkplug B **Primary Application** (SCADA host). It does **not**
authenticate or identify a producer by its MQTT `clientId`. It cannot: an MQTT
**PUBLISH packet does not carry the publishing client's id**, so a subscriber
never sees who sent a message. The producer's `clientId` (e.g. goMqttModbus's
`"EDGE"`) and the gateway's own `cfg.ClientID` are only used for *their own*
broker connections.

The **only** identity goGateway has is the **Sparkplug topic**:

```
spBv1.0/{group_id}/{message_type}/{edge_node_id}[/{device_id}]
```

- Session state is keyed on `NodeKey{GroupID, EdgeNodeID}` + `DeviceID`
  (`internal/sparkplug/session.go`).
- Autodiscovery rows are keyed `UNIQUE(group_id, node_id, device_id)`
  (`autodiscovered_entities`).
- The catalog equipo is keyed `UNIQUE(nombre_topic)` = `group/node[/device]`
  (`ssfv.tbl_equipo`, migration 007).

**Consequence:** identity == the topic triple. Anything that can publish to
`spBv1.0/{group}/#` *is* that node, as far as the gateway is concerned.

### What about `bdSeq`?

Sparkplug's session-continuity mechanism is the **birth/death sequence**
(`bdSeq`): each NBIRTH carries one, and the node's registered Last-Will NDEATH
carries the matching value, so a broker-delivered LWT proves *which* session
died. goGateway **captures** `bdSeq` from NBIRTH (`session.go`) but currently
**does not validate it** — it is not compared against NDEATH, nor required to
advance on re-birth. So bdSeq is not (yet) a defense here.

---

## 2. Failure modes with many / duplicate devices

| Scenario | What goGateway sees | Outcome |
|---|---|---|
| **Add devices** (new `node`/`device`) | a distinct topic triple | clean — own session, own autodiscovery row, no interference |
| **Duplicate device** (2 producers, same `group/node/device`, different clientId) | one `NodeSession`, one seq counter | **collision**: interleaved seqs never validate → **rebirth storm**; both map to the **same** equipo (`nombre_topic` UNIQUE) → data silently merged/deduped |
| **Signal lost** (producer drops) | connection loss / LWT NDEATH | graceful — points marked stale (`QualityNotTopical`), ingest stops; history kept |

The dangerous one is the **duplicate**. Sparkplug keeps **one seq counter per
EoN node** (shared across NBIRTH/NDATA/DBIRTH/DDATA). Two producers under the
same node id each run their own counter starting at 0; the gateway sees the
single shared session bounce between them, every other message is
out-of-sequence, and each gap triggers an NCMD Rebirth that the *other* producer
immediately invalidates again — a self-sustaining storm. Meanwhile both write to
the same `equisenal_id`, so `tbl_valores`' `ON CONFLICT (timestamp, equisenal)
DO NOTHING` silently drops whichever sample loses the race.

### Reproduced — `scripts/soak-edgecases.sh`

```
A. ADD DEVICES      edge-2 + meter-02 appear as NEW 'pending' rows,
                    independent of the approved edge-1/meter-01.        ✓ clean

B. DUPLICATED       2nd producer, clientId=edge-dup, SAME topic
                    EPM_SOAK/edge-1/meter-01:
                      Δ out-of-sequence ≈ 18 / 30s   (rebirth storm)
                      Δ rebirth requests ≈ 18 / 30s
                      equipos for that topic = 1      (silently merged)
                    clientId is invisible — identity is topic-only.     ⚠ collision

C. SIGNAL LOST      edge-1 stopped → NDEATH delivered, points marked
                    stale, ingest delta → 0.                            ✓ graceful
```

---

## 3. Mitigation strategy (defense in depth)

**1. Broker-level — the real control (do this).** goGateway *cannot* enforce
sender identity; the broker must. Per Sparkplug security guidance:
- Unique credentials per edge node (no shared accounts).
- **Topic ACLs** so a client may only publish to its own
  `spBv1.0/{group}/+/{node}/#`. This makes a duplicate/spoofed node id
  impossible at the source.
- **mTLS** client certificates binding a node id to a cert.
- The gateway runs as a read-mostly Primary App (publishes only `STATE` and
  `NCMD`); restrict those too.

**2. Application-level detection in goGateway (defense in depth — implemented).**
A duplicate cannot be *prevented* here, but it can be *surfaced* instead of
silently corrupting data: `publishRebirth` now runs `detectRebirthStorm`, which
watches the per-node rebirth-request rate and logs a rate-limited **WARNING**
naming the likely cause (duplicate node id) when a sustained storm is detected
(see `internal/mqtt/client.go`). This converts the silent collision into an
actionable operator signal.

**3. Possible future hardening (not yet implemented).**
- Validate `bdSeq`: track it per node, require monotonic advance on re-birth,
  and flag an NBIRTH whose bdSeq regresses while the session is online.
- Surface the storm warning as an SSFV alarm / UI banner, not just a log line.
- Optional "node lock": after first NBIRTH, pin an expected source fingerprint
  (only meaningful alongside broker-side identity, e.g. via cert CN propagated
  by the broker).
