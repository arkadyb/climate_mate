package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/arkadyb/climate_mate/internal/pkg/app"
	"github.com/arkadyb/climate_mate/internal/pkg/rest"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Server struct {
	*http.Server
}

func NewServer(
	version string,
	port string,
	app *app.App,
) *Server {
	router := mux.NewRouter()
	// health endpoint
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{ "status": "SERVING" }`)
	}).Methods("GET")

	// version endpoint
	router.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{ "version": "%s" }`, version)
	}).Methods("GET")

	versionRouter := router.PathPrefix("/v1").Subrouter()
	versionRouter.Handle("/upload",
		rest.DocumentUploadEndpoint(app),
	).Methods("POST")
	versionRouter.Handle("/search",
		rest.DocumentSearchEndpoint(app),
	).Methods("GET")
	versionRouter.Handle("/query",
		rest.QueryEndpoint(app),
	).Methods("GET")

	// default landing page
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./www"))).Methods("GET")

	return &Server{
		Server: &http.Server{
			Addr:    fmt.Sprintf(":%s", port),
			Handler: handlers.LoggingHandler(log.StandardLogger().Writer(), router),
		},
	}
}

func (s *Server) Stop() {
	log.Info("gracefully stopping HTTP server...")
	if err := s.Shutdown(context.Background()); err != nil {
		log.Error(errors.Wrap(err, "failed to gracefully stop HTTP server"))
		return
	}
	log.Info("HTTP server has stopped.")
}

func (s *Server) Start() {
	log.Printf("Starting server on %s", s.Addr)
	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Fatal("Unable to start server: ", err)
			time.Sleep(time.Second)
		}
	}()
}
