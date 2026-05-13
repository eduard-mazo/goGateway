package tsdb

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

const equiSenalNotFound = -1

// SSFVCache resolves signal_path → EquiSenal_Id via ssfv.v_Senales_Contexto.
// Results are cached in memory (lazy, permanent for the lifetime of the adapter).
// Call Invalidate() to flush after catalog changes in the UI.
type SSFVCache struct {
	pool  *pgxpool.Pool
	cache sync.Map // string → int
}

func newSSFVCache(pool *pgxpool.Pool) *SSFVCache {
	return &SSFVCache{pool: pool}
}

// Resolve returns the EquiSenal_Id for a signal_path, or (0, false) if not found.
// The lookup matches "Nombre_Topic/Nombre_Instancia" against signal_path.
func (c *SSFVCache) Resolve(ctx context.Context, signalPath string) (int, bool) {
	if v, ok := c.cache.Load(signalPath); ok {
		id := v.(int)
		return id, id != equiSenalNotFound
	}

	var id int
	err := c.pool.QueryRow(ctx, `
		SELECT sxe."EquiSenal_Id"
		FROM ssfv."Tbl_Equipo" e
		JOIN ssfv."Tbl_Senales_x_Equipo" sxe ON sxe."Equipo_Id" = e."Equipo_Id"
		WHERE e."Nombre_Topic" || '/' || sxe."Nombre_Instancia" = $1
		  AND sxe."Activo" = TRUE
		LIMIT 1`, signalPath).Scan(&id)

	if err != nil {
		log.Printf("ssfv_cache: %q → not found (%v)", signalPath, err)
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
