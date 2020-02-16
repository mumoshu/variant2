package slack

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/nlopes/slack"
	"github.com/rs/xid"
)

const (
	// action is used for slack attament action.
	actionSelect = "select"
	actionCancel = "cancel"
)

type InteractionCallbackHandler func(slack.InteractionCallback) (interface{}, error)

type Connection struct {
	// pendingInteractionCallbacks is the map from callback_id to channel to receive interaction callback
	pendingInteractionCallbacks map[string]InteractionCallbackHandler

	pendingInteractionsCallbackMut *sync.Mutex

	// Channel is the Slack channel name with the "#" prefix to post messages
	Channel string

	// VerificationToken is the token to verify interaction callbacks
	VerificationToken string

	// BotUserOAuthAccessToken can be obtained from https://api.slack.com/apps/<APP ID>/oauth?
	BotUserOAuthAccessToken string

	Client *slack.Client

	triggerCmd string

	// channelID let the bot react only on messages posted in the specific channel. Keep this empty to disable this feature
	channelID string

	// botID let the bot react only on messages mentioning the bot by @<bot name>. Keep this empty to disable this feature
	botID string

	HandleSlashCommand func(*Connection, string, slack.SlashCommand) string

	mut *sync.Mutex
}

type Notification struct {
	Text string
}

type Selection struct {
	Options []string
}

func New(triggerCmd, botUserOAuthAccessToken string, verificationToken string, callback func(*Connection, string, slack.SlashCommand) string) *Connection {
	return &Connection{
		triggerCmd:                     triggerCmd,
		BotUserOAuthAccessToken:        botUserOAuthAccessToken,
		HandleSlashCommand:             callback,
		pendingInteractionCallbacks:    map[string]InteractionCallbackHandler{},
		mut:                            &sync.Mutex{},
		pendingInteractionsCallbackMut: &sync.Mutex{},
		VerificationToken:              verificationToken,
	}
}

func (conn *Connection) slackClient(accessToken string) *slack.Client {
	conn.mut.Lock()

	defer conn.mut.Unlock()

	if conn.Client == nil {
		conn.Client = slack.New(accessToken)
	}

	return conn.Client
}

func (conn *Connection) Notify(n Notification) error {
	attachment := slack.Attachment{
		Text:  n.Text,
		Color: "#f9a41b",
	}
	params := slack.PostMessageParameters{
		Markdown: true,
	}

	respChannel, respTs, err := conn.Client.PostMessage(conn.Channel, slack.MsgOptionPostMessageParameters(params), slack.MsgOptionAttachments(attachment))
	if err != nil {
		return err
	}

	fmt.Printf("respCHannel=%s, respTs=%s", respChannel, respTs)

	return nil
}

func newCallbackID() string {
	return xid.New().String()
}

func (conn *Connection) Select(sel Selection) (*string, error) {
	callbackID := newCallbackID()

	// Send selection
	selectOptions := []slack.AttachmentActionOption{}

	for _, o := range sel.Options {
		actionOpt := slack.AttachmentActionOption{
			Text:  o,
			Value: o,
		}
		selectOptions = append(selectOptions, actionOpt)
	}

	attachment := slack.Attachment{
		Text:       "please select one",
		Color:      "#f9a41b",
		CallbackID: callbackID,
		Actions: []slack.AttachmentAction{
			{
				Name:    actionSelect,
				Type:    "select",
				Options: selectOptions,
			},

			{
				Name:  actionCancel,
				Text:  "Cancel",
				Type:  "button",
				Style: "danger",
			},
		},
	}

	params := slack.PostMessageParameters{
		Markdown: true,
	}

	respChannel, respTs, err := conn.Client.PostMessage(conn.Channel, slack.MsgOptionPostMessageParameters(params), slack.MsgOptionAttachments(attachment))
	if err != nil {
		return nil, err
	}

	fmt.Printf("respCHannel=%s, respTs=%s", respChannel, respTs)

	// Wait for selection

	callbackCh := make(chan slack.InteractionCallback, 1)

	conn.RegisterInteractionCallbackHandler(callbackID, func(interaction slack.InteractionCallback) (interface{}, error) {
		callbackCh <- interaction

		return &interaction.OriginalMessage, nil
	})

	interaction := <-callbackCh

	selected := interaction.ActionCallback.AttachmentActions[0].SelectedOptions[0].Value

	return &selected, nil
}

func (conn *Connection) RegisterInteractionCallbackHandler(callbackID string, callback InteractionCallbackHandler) {
	conn.pendingInteractionsCallbackMut.Lock()
	conn.pendingInteractionCallbacks[callbackID] = callback
	conn.pendingInteractionsCallbackMut.Unlock()
}

func (conn *Connection) Run() error {
	handler := func(interaction slack.InteractionCallback) (interface{}, error) {
		callbackID := interaction.CallbackID

		conn.pendingInteractionsCallbackMut.Lock()

		callback, ok := conn.pendingInteractionCallbacks[callbackID]

		conn.pendingInteractionsCallbackMut.Unlock()

		if !ok {
			return nil, fmt.Errorf("no pending selection registered for callback id %q. Probably the bot has restarted since it submitted the interactive message? In that case you just need to redo the action", callbackID)
		}

		res, err := callback(interaction)

		return res, err
	}

	errch := make(chan error)

	rtm := conn.slackClient(conn.BotUserOAuthAccessToken).NewRTM()

	go func() {
		go rtm.ManageConnection()

		for msg := range rtm.IncomingEvents {
			fmt.Print("Event Received: ")

			switch ev := msg.Data.(type) {
			case *slack.HelloEvent:
				fmt.Println("Hello")
			case *slack.ConnectedEvent:
				fmt.Println("Infos:", ev.Info)
				fmt.Println("Connection counter:", ev.ConnectionCount)
			case *slack.MessageEvent:
				fmt.Printf("Message: %v\n", ev)

				if conn.channelID != "" && ev.Channel != conn.channelID {
					return
				}

				if conn.botID != "" && !strings.HasPrefix(ev.Msg.Text, fmt.Sprintf("<@%s> ", conn.botID)) {
					return
				}

				//text := ev.Text
				//
				//if strings.HasPrefix(text, "`/"+conn.triggerCmd) {
				//	cmd := strings.TrimPrefix(text, "`/")
				//	cmd = strings.TrimSuffix(cmd, "`")
				//
				//	res := conn.callback(cmd)
				//
				//	conn.Notify(Notification{Text: res})
				//}

			case *slack.PresenceChangeEvent:
				fmt.Printf("Presence Change: %v\n", ev)

			case *slack.LatencyReport:
				fmt.Printf("Current latency: %v\n", ev.Value)

			case *slack.RTMError:
				fmt.Printf("Error: %s\n", ev.Error())
				errch <- errors.New("slackbot: invalid credential")

				return
			case *slack.InvalidAuthEvent:
				fmt.Printf("Invalid credentials")
				errch <- errors.New("slackbot: invalid credential")

				return
			default:
				// Ignore other events..
				fmt.Printf("Unexpected: %v\n", msg.Data)
			}
		}
	}()

	handler2 := func(cmd slack.SlashCommand) (interface{}, error) {
		fmt.Printf("Slash Command: %v\n", cmd)

		cmdText := strings.Split(cmd.Text, "\n")[0]

		userInput := cmd.Command + " " + cmdText

		// This should be a full Variant command without the executable name.
		//
		// E.g. for what you would type `variant run foo bar --opt1`, this shuold be `run foo bar --opt1`, as without the executable name.
		variantCmdWithArgs := "run " + cmdText

		response := slack.Message{}
		// You should always specify response type even when you're intended to use the default "ephemeral"
		// as per https://api.slack.com/interactivity/slash-commands#responding_to_commands
		response.Type = slack.ResponseTypeEphemeral
		response.Text = fmt.Sprintf("Processing %q", userInput)
		response.Attachments = []slack.Attachment{}

		go func() {
			// I wish we could update this ephemeral message later but it isn't supported.
			//
			// > "Ephemeral messages can't be updated in this way."
			// > https://api.slack.com/messaging/modifying
			//
			// We instead send a separate non-ephemeral message as well and update it.
			_, _, _, err := conn.Client.SendMessage(
				cmd.ChannelID,
				slack.MsgOptionResponseURL(cmd.ResponseURL, slack.ResponseTypeEphemeral),
				slack.MsgOptionText(fmt.Sprintf("Running `%s`... I'll soon post a message visible to everyone in this channel to share the progress and the result of it.", variantCmdWithArgs), false),
			)
			if err != nil {
				fmt.Printf("async response 1 error: %v", err)

				return
			}

			_, ts, _, err := conn.Client.SendMessage(
				cmd.ChannelID,
				// Commented-out as this resulted in mesage_not_found error in the later phase, which seemed to indicate that
				// we cant update the non-ephemeral response after it's posted.
				//slack.MsgOptionResponseURL(cmd.ResponseURL, slack.ResponseTypeInChannel),
				slack.MsgOptionText(fmt.Sprintf("@%s triggered `%s`", cmd.UserName, userInput), false),
			)
			if err != nil {
				fmt.Printf("async response 2 error: %v", err)

				return
			}

			res := conn.HandleSlashCommand(conn, variantCmdWithArgs, cmd)

			fmt.Printf("async slash command run finished: %s\n", res)

			text := fmt.Sprintf("@%s triggered `%s`:\n```\n%s```", cmd.UserName, userInput, res)

			_, _, _, err = conn.Client.UpdateMessage(
				cmd.ChannelID,
				ts,
				slack.MsgOptionText(text, false),
				// Otherwise you might get message_not_found error
				// We don't use MsgOptionPostEphemeral as it results in posting a new message instead of updating the existing one
				// slack.MsgOptionUser(cmd.UserID),
			)
			if err != nil {
				fmt.Printf("async response 3 error: %v", err)
				return
			}
		}()

		return &response, nil
	}

	slaHandler := slackInteractionsToHTTPHandler(handler, handler2, conn.VerificationToken)

	go func() {
		errch <- http.ListenAndServe(":8080", slaHandler)
	}()

	return <-errch
}
