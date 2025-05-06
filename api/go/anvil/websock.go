package anvil

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

type Websock struct {
	conn     *websocket.Conn
	handlers WebsockHandlers
}

type WebsockHandlers struct {
	Notification func(n *Notification, err error)
}

func (ws *Websock) Run() error {
	for {
		typ, buf, err := ws.conn.ReadMessage()
		if err != nil {
			return err
		}

		if typ != websocket.TextMessage {
			continue
		}

		var n Notification
		err = json.Unmarshal(buf, &n)
		if ws.handlers.Notification != nil {
			ws.handlers.Notification(&n, err)
		}
	}
}
