package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
)

var Config struct {
	Debug         bool
	SlackToken    string
	SlackTeam     string
	WebAddress    string
	MinecraftHost string
	MinecraftPort int
}

var MinecraftAddress *net.TCPAddr

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

type MinecraftStatus struct {
	ProtocolVersion uint64
	ServerVersion   string
	Motd            string
	Players         uint64
	MaxPlayers      uint64
}

func GetStatus() (stat *MinecraftStatus, err error) {
	log.Println("Connecting to", MinecraftAddress)
	netConn, err := net.DialTCP("tcp", nil, MinecraftAddress)
	if err != nil {
		return
	}
	defer netConn.Close()
	log.Println("Connected!")
	conn := bufio.NewReadWriter(bufio.NewReader(netConn), bufio.NewWriter(netConn))

	// Send a 1.7 Server List Ping
	// http://wiki.vg/Server_List_Ping (mostly)
	data := make([]byte, 256)

	err = binary.Write(conn, binary.BigEndian, uint8(0x0F))

	n := binary.PutUvarint(data, 0)
	err = binary.Write(conn, binary.BigEndian, data[:n])
	if err != nil {
		return
	}

	// Server List says to use 4 but 1.7.10 is actually 5.
	n = binary.PutUvarint(data, 5)
	err = binary.Write(conn, binary.BigEndian, data[:n])
	if err != nil {
		return
	}

	n = binary.PutUvarint(data, uint64(len(Config.MinecraftHost)))
	err = binary.Write(conn, binary.BigEndian, data[:n])
	if err != nil {
		return
	}
	err = binary.Write(conn, binary.BigEndian, []byte(Config.MinecraftHost))
	if err != nil {
		return
	}

	err = binary.Write(conn, binary.BigEndian, uint16(Config.MinecraftPort))
	if err != nil {
		return
	}

	n = binary.PutUvarint(data, 1)
	err = binary.Write(conn, binary.BigEndian, data[:n])
	if err != nil {
		return
	}
	// ??? but minecraft does it
	err = binary.Write(conn, binary.BigEndian, data[:n])
	if err != nil {
		return
	}

	n = binary.PutUvarint(data, 0)
	err = binary.Write(conn, binary.BigEndian, data[:n])
	if err != nil {
		return
	}

	conn.Flush()
	log.Println("Wrote a bunch of junk, about to read...")

	info := make(map[string]interface{})

	// Just throw away five bytes.
	for i := 0; i < 5; i++ {
		_, err = conn.ReadByte()
		if err != nil {
			return
		}
	}

	dec := json.NewDecoder(conn)
	err = dec.Decode(&info)
	if err != nil {
		return
	}

	//err = fmt.Errorf("LOL TLDR")
	version := info["version"].(map[string]interface{})
	players := info["players"].(map[string]interface{})
	stat = &MinecraftStatus{
		ProtocolVersion: uint64(version["protocol"].(float64)),
		ServerVersion:   version["name"].(string),
		Motd:            info["description"].(string),
		Players:         uint64(players["online"].(float64)),
		MaxPlayers:      uint64(players["max"].(float64)),
	}
	return
}

func StatusReport(msg *SlackMessage) (text string, err error) {
	stat, err := GetStatus()
	if err != nil {
		return
	}

	text = fmt.Sprintf("*%s* has *%d*/%d players on.", stat.Motd, stat.Players, stat.MaxPlayers)
	return
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
		if req.Method == "GET" {
			text, err := StatusReport(nil)
			if err != nil {
				http.Error(w, err.Error(), 500)
			} else {
				fmt.Fprintf(w, "%s", text)
			}
			return
		}

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

	address := fmt.Sprintf("%s:%d", Config.MinecraftHost, Config.MinecraftPort)
	MinecraftAddress, err = net.ResolveTCPAddr("tcp", address)
	if err != nil {
		log.Println("Error resolving Minecraft address", address, ":", err.Error())
		return
	}

	// Try immediately if we're in debug mode.
	if Config.Debug {
		stat, err := GetStatus()
		if err != nil {
			log.Println(err.Error())
			return
		}
		log.Println(stat.Motd)
	}

	http.ListenAndServe(Config.WebAddress, nil)
}
