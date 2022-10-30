package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/net/websocket"
)

type Participant struct {
	Room    *Room       `json:"-"`
	Display string      `json:"display"`
	Out     chan []byte `json:"-"`
}

type Room struct {
	Name         string         `json:"room"`
	Participants []*Participant `json:"participants"`
	lock         sync.RWMutex   `json:"-"`
}

func (v *Room) Add(p *Participant) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	for _, r := range v.Participants {
		if r.Display == p.Display {
			return errors.New("display existing")
		}
	}

	v.Participants = append(v.Participants, p)
	return nil
}

func (v *Room) Get(display string) *Participant {
	v.lock.RLock()
	defer v.lock.RUnlock()

	for _, r := range v.Participants {
		if r.Display == display {
			return r
		}
	}

	return nil
}

func (v *Room) Remove(p *Participant) {
	v.lock.Lock()
	defer v.lock.Unlock()

	for i, r := range v.Participants {
		if p == r {
			v.Participants = append(v.Participants[:i], v.Participants[i+1:]...)
			return
		}
	}
}

func (v *Room) Notify(ctx context.Context, peer *Participant, event, data string) {
	var participants []*Participant

	for _, r := range participants {
		if r == peer {
			continue
		}

		res := struct {
			Action string `json:"action"`
			Event  string `json:"event"`
			Data   string `json:"data,omitempty"`
		}{
			"notify", event, data,
		}

		b, err := json.Marshal(struct {
			Message interface{} `json:"msg"`
		}{
			res,
		})
		if err != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case r.Out <- b:
		}

		fmt.Println("Notify %v about %v %v", r, peer, event)
	}
}

func main() {
	ctx := context.Background()

	http.Handle("/sig", websocket.Handler(func(c *websocket.Conn) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		r := c.Request()
		defer c.Close()
	}))
}
