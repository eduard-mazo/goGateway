package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"

	"goGateway/internal/auth"
	"goGateway/internal/iec104"
	"goGateway/internal/mqtt"
	"goGateway/internal/tsdb"
	"goGateway/internal/web"
)

// Deps wire handlers to their notifiers.
type Deps struct {
	DB             *sqlx.DB
	NotifyMQTT     func() // reload MQTT client on cfg/topic change
	NotifyIEC104   func() // reload IEC 104 server on cfg change
	NotifyMappings func() // reload mapping cache on mapping change
	NotifyTSDB     func() // reload TSDB pipeline on cfg change
	NotifyNATS     func() // reload NATS/workers on cfg change
	NotifySSFV     func() // reload worker SSFVMappingCache on catalog change

	MQTT      *mqtt.Manager
	IEC104    iec104.Server
	TSDBMgr   *tsdb.Manager
	BrokerMon *BrokerMonitor
	StartedAt time.Time

	// AuthCfg configures JWT signing and token lifetimes.
	// When set, /api/auth/* routes are mounted.
	// To protect ALL API routes add auth.AuthMiddleware(AuthCfg, sessions)
	// to the r.Group below.
	AuthCfg auth.Config
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMW)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	r.Route("/api", func(r chi.Router) {
		// SSE stream — long-lived, exempt from the 30s timeout.
		if d.BrokerMon != nil {
			r.Get("/broker/stream", d.BrokerMon.ServeStream)
		}

		// Auth routes (public — no token required for login/refresh).
		if len(d.AuthCfg.Secret) > 0 {
			authH := NewAuthHandler(d, d.AuthCfg)
			r.Route("/auth", authH.Mount)
		}

		// All other API routes: 30s timeout + 1 MB body limit.
		// To require authentication for ALL routes uncomment:
		//   r.Use(auth.AuthMiddleware(d.AuthCfg, auth.NewSessionStore(d.DB)))
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Second))
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
					next.ServeHTTP(w, r)
				})
			})

			if d.BrokerMon != nil {
				r.Get("/broker/events", d.BrokerMon.ServeSnapshot)
			}

			r.Route("/devices", (&DeviceHandler{DB: d.DB}).Mount)
			r.Route("/topics", (&TopicHandler{DB: d.DB, Notify: d.NotifyMQTT}).Mount)
			r.Route("/mqtt-config", (&MQTTConfigHandler{DB: d.DB, Notify: d.NotifyMQTT}).Mount)
			r.Route("/nats-config", (&NATSConfigHandler{DB: d.DB, Notify: d.NotifyNATS}).Mount)
			r.Route("/iec104-gateway", (&IEC104GatewayHandler{DB: d.DB, Notify: d.NotifyIEC104}).Mount)
			r.Route("/iec104-servers", (&IEC104ServersHandler{DB: d.DB, Notify: d.NotifyIEC104}).Mount)
			r.Route("/mappings", (&MappingHandler{DB: d.DB, Notify: d.NotifyMappings}).Mount)
			r.Route("/history", (&HistoryHandler{DB: d.DB}).Mount)
			r.Route("/status", (&StatusHandler{DB: d.DB, MQTT: d.MQTT, IEC104: d.IEC104, StartedAt: d.StartedAt}).Mount)
			r.Route("/tsdb-config", (&TSDBConfigHandler{DB: d.DB, Notify: d.NotifyTSDB}).Mount)
			tsdbH := tsdb.NewHandler(d.TSDBMgr)
			r.Get("/tsdb/status", tsdbH.ServeStatus)
			r.Get("/tsdb/dlq", tsdbH.ServeDLQ)
			r.Post("/tsdb/dlq/replay", tsdbH.ServeDLQReplay)
			r.Get("/tsdb/points", tsdbH.ServePoints)

			ssfvApiH := NewSSFVHandler(d.TSDBMgr)
			ssfvApiH.SetReloader(d.NotifySSFV)
			r.Route("/ssfv", ssfvApiH.Mount)
		})
	})

	// Embedded SPA — serves frontend/dist bundled into the binary.
	// Must mount last so /api and /health take precedence.
	r.Mount("/", web.Handler())
	return r
}

func corsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
