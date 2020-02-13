package slack

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/nlopes/slack"
)

type InteractionsHandler func(callback slack.InteractionCallback) (interface{}, error)

type interactionsHTTPHandler struct {
	VerificationToken string

	handler InteractionsHandler
}

func (h interactionsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("[ERROR] Invalid method: %s", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)

		return
	}

	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	jsonStr, err := url.QueryUnescape(string(buf)[8:])
	if err != nil {
		log.Printf("[ERROR] Failed to unespace request body: %s", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	var message slack.InteractionCallback
	if err := json.Unmarshal([]byte(jsonStr), &message); err != nil {
		log.Printf("[ERROR] Failed to decode json message from slack: %s", jsonStr)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	// Only accept message from slack with valid token
	if message.Token != h.VerificationToken {
		log.Printf("[ERROR] Invalid token: %s", message.Token)
		w.WriteHeader(http.StatusUnauthorized)

		return
	}

	output, err := h.handler(message)
	if err != nil {
		log.Printf("[ERROR] handler failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)

		return
	}

	w.Header().Add("Content-type", "application/json")
	w.WriteHeader(http.StatusOK)

	if output == nil {
		fmt.Fprintf(w, "")

		return
	}

	if err := json.NewEncoder(w).Encode(&output); err != nil {
		panic(err)
	}
}

func slackInteractionsToHTTPHandler(handler InteractionsHandler, handler2 func(cmd slack.SlashCommand) (interface{}, error), verificationToken string) HTTPHandler {
	mux := http.NewServeMux()
	mux.Handle("/interactions", interactionsHTTPHandler{
		VerificationToken: verificationToken,
		handler:           handler,
	})
	mux.Handle("/slashcommands", slashCommandsHTTPHandler{
		VerificationToken: verificationToken,
		handler:           handler2,
	})

	return mux
}
