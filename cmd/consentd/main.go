package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/example/devtools-consent-gate/internal/consent"
)

type server struct {
	client *consent.Client
}

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = ":8080"
	}

	s := &server{client: consent.NewClient(apiKey)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /operations/authorize", s.authorize)
	mux.HandleFunc("POST /consent/changes", s.changeConsent)
	log.Printf("consentd listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}

func (s *server) authorize(w http.ResponseWriter, r *http.Request) {
	var operation consent.Operation
	if err := decodeJSON(w, r, &operation); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	decision, err := consent.AuthorizeOperation(r.Context(), s.client, operation)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decision)
}

func (s *server) changeConsent(w http.ResponseWriter, r *http.Request) {
	var change consent.Change
	if err := decodeJSON(w, r, &change); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := consent.ApplyChange(r.Context(), s.client, change)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("request body must be one JSON object with known fields")
	}
	return nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	var inputErr *consent.InputError
	if errors.As(err, &inputErr) {
		writeError(w, http.StatusBadRequest, inputErr)
		return
	}
	var apiErr *consent.APIError
	if errors.As(err, &apiErr) {
		writeJSON(w, consent.StatusForError(err), map[string]any{
			"error": map[string]any{"code": apiErr.Code, "message": apiErr.Message},
		})
		return
	}
	if consent.StatusForError(err) == http.StatusBadGateway {
		writeError(w, http.StatusBadGateway, errors.New("upstream request failed"))
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
