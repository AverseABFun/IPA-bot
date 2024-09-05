package main

import (
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
)

var api *slack.Client

type ReactionMessage struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	ID      string `json:"ts"`
}

type EventStruct struct {
	Type     string          `json:"type"`
	Reaction string          `json:"reaction"`
	Message  ReactionMessage `json:"item"`
	Text     string          `json:"text"`
	Channel  string          `json:"channel"`
	ID       string          `json:"ts"`
}

type EventWrapper struct {
	Type      string      `json:"type"`
	Challenge string      `json:"challenge"`
	Event     EventStruct `json:"event"`
}

type IPALanguage struct {
	LangID      string
	WordsToIPAs map[string]string
}

func events(w http.ResponseWriter, r *http.Request) {
	/*var verifier, err = slack.NewSecretsVerifier(r.Header, os.Getenv("SIGNING_SECRET"))
	if err != nil {
		log.Fatalf("Error creating secrets verifier: %s", err)
	}

	if err := verifier.Ensure(); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Fatalf("Error validating slack secret: %s", err)
	}*/

	var event = EventWrapper{}
	decodeJSONBody(w, r, &event)

	if event.Type == "url_verification" {
		w.Write([]byte(event.Challenge))
	} else if event.Type == "event_callback" {
		switch event.Event.Type {
		case "reaction_added":
			if event.Event.Reaction == "ipa" {
				switch event.Event.Message.Type {
				case "message":
					messages, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: event.Event.Message.Channel, Latest: event.Event.Message.ID, Oldest: event.Event.Message.ID, Inclusive: true})
					if err != nil {
						log.Fatalf("Error reading message: %s", err)
					}
					message := messages.Messages[0]
					text := strings.TrimSpace(message.Text)
					var re = regexp.MustCompile(`(?m)\s*<@[A-Z0-9]*?>\s*`)
					text = re.ReplaceAllString(text, "")
					words := strings.Split(text, " ")
					ipas := []string{}
					var found = false
					var set = false
					for _, word := range words {
						word = strings.ToLower(word)
						found = false
						var tempFound = false
						for _, lang := range loadedLanguages {
							var val, ok = lang.WordsToIPAs[word]
							if !ok {
								continue
							}
							ipas = append(ipas, val+" ("+lang.LangID+")")
							tempFound = true
							if set {
								break
							}
							set = true
							found = true
							break
						}
						if !tempFound {
							ipas = append(ipas, "???")
						}
					}
					var fullIPA = "/" + strings.Join(ipas, " ") + "/"
					if !found {
						api.PostMessage(event.Event.Message.Channel, slack.MsgOptionTS(event.Event.Message.ID), slack.MsgOptionText("Got partial IPA! Try loading another language with /load. Unknown IPAs have been replaced with \"???\". "+fullIPA, true))
					} else {
						api.PostMessage(event.Event.Message.Channel, slack.MsgOptionTS(event.Event.Message.ID), slack.MsgOptionText(fullIPA, true))
					}
				}
			}
		case "app_mention":
			messages, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: event.Event.Channel, Latest: event.Event.ID, Oldest: event.Event.ID, Inclusive: true})
			if err != nil {
				log.Fatalf("Error reading message: %s", err)
			}
			message := messages.Messages[0]
			text := strings.TrimSpace(message.Text)
			var re = regexp.MustCompile(`(?m)\s*<@[A-Z0-9]*?>\s*`)
			text = re.ReplaceAllString(text, "")
			words := strings.Split(text, " ")
			ipas := []string{}
			var found = false
			var set = false
			for _, word := range words {
				word = strings.ToLower(word)
				found = false
				var tempFound = false
				for _, lang := range loadedLanguages {
					var val, ok = lang.WordsToIPAs[word]
					if !ok {
						continue
					}
					ipas = append(ipas, val+" ("+lang.LangID+")")
					tempFound = true
					if set {
						break
					}
					set = true
					found = true
					break
				}
				if !tempFound {
					ipas = append(ipas, "???")
				}
			}
			var fullIPA = "/" + strings.Join(ipas, " ") + "/"
			if !found {
				api.PostMessage(event.Event.Channel, slack.MsgOptionTS(event.Event.ID), slack.MsgOptionText("Got partial IPA! Try loading another language with /load. Unknown IPAs have been replaced with \"???\". "+fullIPA, true))
			} else {
				api.PostMessage(event.Event.Channel, slack.MsgOptionTS(event.Event.ID), slack.MsgOptionText(fullIPA, true))
			}
		}
	}
	w.WriteHeader(http.StatusOK)
}

func load(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Fatalf("Error parsing form encoded data: %s", err)
	}
	switch r.Form.Get("command") {
	case "/load":
		if strings.ContainsAny(r.Form.Get("text"), "/\\") {
			slack.PostWebhook(r.Form.Get("response_url"), &slack.WebhookMessage{Text: "Trying to break out, I see?"})
			break
		}
		slack.PostWebhook(r.Form.Get("response_url"), &slack.WebhookMessage{Text: "Loading..."})
		lang, err := readLanguage(r.Form.Get("text"))
		if err != nil {
			log.Printf("Error reading language: %s\n", err)
			slack.PostWebhook(r.Form.Get("response_url"), &slack.WebhookMessage{Text: "Got an error: " + err.Error()})
			break
		}
		loadedLanguages = append(loadedLanguages, lang)
		slack.PostWebhook(r.Form.Get("response_url"), &slack.WebhookMessage{Text: "Loaded!"})
	}
	w.WriteHeader(http.StatusOK)
}

var loadedLanguages []*IPALanguage

func readLanguage(langName string) (*IPALanguage, error) {
	var path = "./ipa-dict/data/" + langName + ".txt"
	var _, err = os.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var out = IPALanguage{}
	out.LangID = langName
	out.WordsToIPAs = make(map[string]string)

	for _, str := range strings.Split(string(f), "\n") {
		if strings.TrimSpace(str) == "" {
			continue
		}
		var fields = strings.Fields(str)
		var word = strings.ToLower(fields[0])
		fields = fields[1:]
		var IPA = strings.Join(fields, "")
		IPA = strings.Split(IPA, ",")[0]
		IPA = strings.Trim(IPA, "/")
		out.WordsToIPAs[word] = IPA
	}
	return &out, nil
}

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}

	lang, err := readLanguage("en_US")
	if err != nil {
		log.Fatalf("Error reading language: %s", err)
	}
	loadedLanguages = append(loadedLanguages, lang)

	api = slack.New(os.Getenv("TOKEN"), slack.OptionDebug(true))

	s := &http.Server{
		Addr: ":40277",
	}
	http.HandleFunc("/events", events)
	http.HandleFunc("/load", load)
	log.Fatal(s.ListenAndServe())
}
