package slack

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/nlopes/slack"
)

// See https://api.slack.com/interactivity/slash-commands
type slashCommandsHTTPHandler struct {
	VerificationToken string

	handler func(callback slack.SlashCommand) (interface{}, error)
}

func (h slashCommandsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[ERROR] Invalid method: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	cmd, err := slack.SlashCommandParse(r)
	if err != nil {
		log.Printf("[ERROR] failed to parse slash command: %v", err)
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	// Only accept message from slack with valid token
	if cmd.Token != h.VerificationToken {
		log.Printf("[ERROR] Invalid token: %s", cmd.Token)
		w.WriteHeader(http.StatusUnauthorized)

		return
	}

	output, err := h.handler(cmd)
	if err != nil {
		log.Printf("[ERROR] handler failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Add("Content-type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(&output); err != nil {
		panic(err)
	}
}
