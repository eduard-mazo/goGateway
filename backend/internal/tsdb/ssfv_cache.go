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

// Resolve returns the equisenal_id for a signal_path, or (0, false) if not found.
// The lookup matches "nombre_topic/nombre_instancia" against signal_path.
func (c *SSFVCache) Resolve(ctx context.Context, signalPath string) (int, bool) {
	if v, ok := c.cache.Load(signalPath); ok {
		id := v.(int)
		return id, id != equiSenalNotFound
	}

	var id int
	err := c.pool.QueryRow(ctx, `
		SELECT sxe.equisenal_id
		FROM ssfv.tbl_equipo e
		JOIN ssfv.tbl_senales_x_equipo sxe ON sxe.equipo_id = e.equipo_id
		WHERE e.nombre_topic || '/' || sxe.nombre_instancia = $1
		  AND sxe.activo = TRUE
		LIMIT 1`, signalPath).Scan(&id)

	if err != nil {
		c.cache.Store(signalPath, equiSenalNotFound)
		return 0, false
	}
	c.cache.Store(signalPath, id)
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
