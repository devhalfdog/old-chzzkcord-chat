package chzzkchat

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

const (
	chzzkSocketURL = "wss://kr-ss%d.chat.naver.com/chat"
	socketFirstNum = 1
	socketLastNum  = 10

	pingMessage  = `{"ver":"2","cmd":0}`
	pongMessage  = `{"ver":"2","cmd":10000}`
	setupMessage = `{"ver":"2","cmd":100,"svcid":"game","cid":"%s","bdy":{"uid":"%s","devType":2001,"accTkn":"%s","auth":"SEND"},"tid":1}`
	loginMessage = `{"ver":"2","cmd":5101,"svcid":"game","cid":"%s","sid":"%s","bdy":{"recentMessageCount":50},"tid":2}`
)

type Client struct {
	Token Token

	socketAddress string
	socket        *websocket.Conn
	read          chan string
	onChatMessage func(messages []ChatMessage)
}

type Token struct {
	Access    string
	UserID    string
	ChannelID string
}

type ChatMessage struct {
	User User
}

type User struct {
	Hash           string
	Nickname       string
	UserRole       string
	Badge          string
	Title          string
	Verified       bool
	ActivityBadges []Badges
	Message        string
	Donation       Donation
}

type Badges struct {
	Title string
}

type Donation struct {
	Amount string // 사용 금액
}

func (c *Client) OnChatMessage(callback func(messages []ChatMessage)) {
	c.onChatMessage = callback
}

func NewClient(token Token) *Client {
	return &Client{
		socketAddress: "",
		Token:         token,
		read:          make(chan string, 64),
	}
}

func (c *Client) IsConnection() bool {
	return c.socket != nil
}

func (c *Client) Connect() error {
	err := c.createWebSocket()
	if err != nil {
		return err
	}

	return c.makeConnection()
}

func (c *Client) makeConnection() (err error) {
	wg := sync.WaitGroup{}

	wg.Add(1)
	go c.startReader(&wg)

	c.setupConnection()
	c.sendPing()

	err = c.startParser()
	if err != nil {
		return err
	}

	wg.Wait()

	c.socket.Close()
	return
}

func (c *Client) startReader(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		_, message, err := c.socket.ReadMessage()
		if err != nil {
			c.read <- "error: " + err.Error()
			return
		}

		c.read <- string(message)
	}
}

func (c *Client) startParser() error {
	for msg := range c.read {
		if strings.HasPrefix(msg, "error: ") {
			return errors.New(msg)
		}

		cmd := gjson.Get(msg, "cmd").Int()

		switch {
		case isPingMessage(cmd):
			c.socket.WriteMessage(websocket.TextMessage, []byte(pongMessage))
		case isLoginRequiredMessage(cmd):
			sid := gjson.Get(msg, "bdy.sid").String()
			c.socket.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(loginMessage, c.Token.ChannelID, sid)))
		case isChatMessage(cmd) || isDonationMessage(cmd):
			p := c.parseChatMessage(msg, cmd)
			c.onChatMessage(*p)
		}
	}

	return nil
}

func (c *Client) parseChatMessage(msg string, chatType int64) *[]ChatMessage {
	// chatType
	// 93101 -> Chat
	// 93102 -> Donation
	bodies := gjson.Get(msg, "bdy")
	var messages []ChatMessage = make([]ChatMessage, 0)

	bodies.ForEach(func(key, value gjson.Result) bool {
		var message ChatMessage
		message.User.Message = value.Get("msg").String()

		profile := value.Get("profile").String()
		message.User.ActivityBadges = []Badges{}
		message.User.UserRole = gjson.Get(profile, "userRoleCode").String()
		message.User.Hash = gjson.Get(profile, "userIdHash").String()
		message.User.Verified = gjson.Get(profile, "verifiedMark").Bool()
		message.User.Title = gjson.Get(profile, "title").String()
		message.User.Nickname = gjson.Get(profile, "nickname").String()
		message.User.Badge = gjson.Get(profile, "badge").String()

		gjson.Get(profile, "activityBadges").ForEach(func(key, value gjson.Result) bool {
			var activityBadges Badges
			activityBadges.Title = gjson.Get(value.String(), "title").String()
			message.User.ActivityBadges = append(message.User.ActivityBadges, activityBadges)
			return true
		})

		if chatType == 93102 {
			message.User.Donation.Amount = gjson.Get(value.Get("extras").String(), "payAmount").String()
		}

		messages = append(messages, message)
		return true
	})

	return &messages
}

func (c *Client) setupConnection() {
	setupMsg := fmt.Sprintf(setupMessage, c.Token.ChannelID, c.Token.UserID, c.Token.Access)
	c.socket.WriteMessage(websocket.TextMessage, []byte(setupMsg))
}

func (c *Client) createWebSocket() error {
	if c.socket != nil {
		return nil
	}

	bestServer := getSocketAddr()

	var err error
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 15 * time.Second
	c.socket, _, err = dialer.Dial(bestServer, http.Header{})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) sendPing() {
	t := time.NewTicker(20 * time.Second)
	go func() {
		for {
			select {
			case <-t.C:
				c.socket.WriteMessage(websocket.TextMessage, []byte(pingMessage))
			}
		}
	}()
}

func isPingMessage(cmd int64) bool {
	return cmd == 0
}

func isLoginRequiredMessage(cmd int64) bool {
	return cmd == 10100
}

func isChatMessage(cmd int64) bool {
	return cmd == 93101
}

func isDonationMessage(cmd int64) bool {
	return cmd == 93102
}

func getSocketAddr() string {
	bestTime := time.Duration(math.MaxInt64)
	bestServer := ""

	for i := socketFirstNum; i <= socketLastNum; i++ {
		url := fmt.Sprintf(chzzkSocketURL, i)
		setupTime, roundTripTime, err := testSocketConnection(url)
		if err != nil {
			// fmt.Printf("Error connecting to %s: %v\n", url, err)
			continue
		}

		totalTime := setupTime + roundTripTime
		if totalTime < bestTime {
			bestTime = totalTime
			bestServer = url
		}
	}

	return bestServer
}

func testSocketConnection(url string) (time.Duration, time.Duration, error) {
	dialer := websocket.DefaultDialer

	start := time.Now()
	conn, _, err := dialer.Dial(url, http.Header{})
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()
	setupTime := time.Since(start)

	start = time.Now()
	if err = conn.WriteMessage(websocket.TextMessage, []byte(`{}`)); err != nil {
		return 0, 0, err
	}

	_, _, err = conn.ReadMessage()
	if err != nil {
		return 0, 0, err
	}
	roundTripTime := time.Since(start)

	return setupTime, roundTripTime, nil
}
