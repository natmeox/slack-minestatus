package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

var Config struct {
	Debug      bool
	SlackToken string
	SlackTeam  string
	WebAddress string
	MinecraftAddress string
}

var MinecraftAddress *TCPAddr

type SlackMessage struct {
	ChannelName string
	UserName    string
	UserId      string
	Text        string
	Trigger     string
	Timestamp   string
}

type SlackResponse struct {
	Text string `json:"text"`
}

func StatusReport(msg *SlackMessage) (text string, err error) {
}

func StatusRespond(msg *SlackMessage) (text string, err error) {
	switch strings.ToLower(msg.Text) {
	case "status":
		text, err = StatusReport(msg)
	default:
		text = fmt.Sprintf("The term “%s” is not a known command.", msg.Text)
	}

	if err == nil {
		text = fmt.Sprintf("%s: %s", msg.UserName, text)
	}

	return
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "./config.json", "path to configuration file")

	flag.Parse()

	configFile, err := os.Open(configPath)
	if err != nil {
		log.Println(err)
		return
	}
	dec := json.NewDecoder(configFile)
	err = dec.Decode(&Config)
	if err != nil {
		log.Println("Error decoding configuration:", err)
		return
	}

	http.HandleFunc("/jack/", func(w http.ResponseWriter, req *http.Request) {
		trigger := req.PostFormValue("trigger_word")
		fullText := req.PostFormValue("text")
		text := strings.TrimSpace(strings.TrimPrefix(fullText, trigger))

		msg := &SlackMessage{
			ChannelName: req.PostFormValue("channel_name"),
			UserName:    req.PostFormValue("user_name"),
			UserId:      req.PostFormValue("user_id"),
			Timestamp:   req.PostFormValue("timestamp"),
			Trigger:     trigger,
			Text:        text,
		}

		ret, err := StatusRespond(msg)
		if err != nil {
			ret = fmt.Sprintf("Oops: %s", err.Error())
		}

		response := &SlackResponse{
			Text: ret,
		}
		responseText, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		w.Header().Set("Content-Type", "text/json")
		w.Write([]byte(responseText))
	})

	MinecraftAddress, err = net.ResolveTCPAddr(Config.MinecraftAddress)
	if err != nil {
		log.Println("Error resolving Minecraft address", Config.MinecraftAddress, ":", err.Error())
		return
	}

	http.ListenAndServe(Config.WebAddress, nil)
}
