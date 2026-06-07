package tsdb

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

const equiSenalNotFound = -1

// SSFVCache resolves signal_path → equisenal_id via ssfv.tbl_senales_x_equipo.
// Results are cached in memory (lazy, permanent for the lifetime of the adapter).
// Call Invalidate() to flush after catalog changes in the UI.
type SSFVCache struct {
	pool  *pgxpool.Pool
	cache sync.Map // string → int
}

func newSSFVCache(pool *pgxpool.Pool) *SSFVCache {
	return &SSFVCache{pool: pool}
}

// Resolve returns the equisenal_id for a metric, or (0, false) if not found.
// C1 composite identity: (entity nombre_topic, codigo_senal, nombre_instancia) —
// the FIWARE Entity → Attribute → channel model.
func (c *SSFVCache) Resolve(ctx context.Context, entity, codigo, instance string) (int, bool) {
	if instance == "" {
		instance = "default"
	}
	key := entity + "\x00" + codigo + "\x00" + instance
	if v, ok := c.cache.Load(key); ok {
		id := v.(int)
		return id, id != equiSenalNotFound
	}

	var id int
	err := c.pool.QueryRow(ctx, `
		SELECT sxe.equisenal_id
		FROM ssfv.tbl_equipo e
		JOIN ssfv.tbl_senales_x_equipo sxe ON sxe.equipo_id = e.equipo_id
		JOIN ssfv.tbl_senales          s   ON s.senal_id   = sxe.senal_id
		WHERE e.nombre_topic = $1 AND s.codigo_senal = $2 AND sxe.nombre_instancia = $3
		  AND sxe.activo = TRUE
		LIMIT 1`, entity, codigo, instance).Scan(&id)

	if err != nil {
		c.cache.Store(key, equiSenalNotFound)
		return 0, false
	}
	c.cache.Store(key, id)
	return id, true
}

// Invalidate flushes all cached entries, forcing re-lookup on next access.
func (c *SSFVCache) Invalidate() {
	c.cache.Range(func(k, _ any) bool {
		c.cache.Delete(k)
		return true
	})
	log.Printf("ssfv_cache: invalidated")
}

// mapQualityToText converts a numeric IEC-104 quality byte to ssfv text.
func mapQualityToText(q int) string {
	switch {
	case q == 0:
		return "Buena"
	case q&0x80 != 0: // QualityInvalid
		return "Mala"
	default: // NotTopical, Substituted, Blocked
		return "Dudosa"
	}
}

// qualityFromTags extracts and converts the "quality" tag to a text value.
func qualityFromTags(tags map[string]string) string {
	if q, ok := tags["quality"]; ok {
		var qi int
		if _, err := fmt.Sscanf(q, "%d", &qi); err == nil {
			return mapQualityToText(qi)
		}
	}
	return "Buena"
}
