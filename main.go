package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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

func (v *Room) Forward(ctx context.Context, peer *Participant, data []byte) {
	var participants []*Participant

	for _, r := range participants {
		if r == peer {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case r.Out <- data:
		}
	}
}

func main() {
	ctx := context.Background()
	var rooms sync.Map

	http.Handle("/sig/v1/p2p", websocket.Handler(func(c *websocket.Conn) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		req := c.Request()
		log.Printf("client %v at %v", req.RemoteAddr, req.RequestURI)
		defer c.Close()

		var self *Participant
		go func() {
			<-ctx.Done()
			if self == nil {
				return
			}

			self.Room.Remove(self)
			log.Printf("Remove client %v", self)
		}()

		inMessages := make(chan []byte, 0)
		go func() {
			defer cancel()

			buf := make([]byte, 16384)
			for {
				n, err := c.Read(buf)
				if err != nil {
					break
				}

				select {
				case <-ctx.Done():
				case inMessages <- buf[:n]:
				}
			}
		}()

		outMessages := make(chan []byte, 0)
		go func() {
			defer cancel()

			handleMessage := func(m []byte) error {
				obj := struct {
					TID     string `json:"tid"`
					Message struct {
						Action  string `json:"action"`
						Room    string `json:"room"`
						Display string `json:"display"`
					} `json:"msg"`
				}{}

				if err := json.Unmarshal(m, &obj); err != nil {
					return errors.New("message format error")
				}

				r, _ := rooms.LoadOrStore(obj.Message.Room, &Room{Name: obj.Message.Room})
				var p *Participant

				if obj.Message.Action == "join" {
					p = &Participant{Room: r.(*Room), Display: obj.Message.Display, Out: outMessages}
					if err := r.(*Room).Add(p); err != nil {
						return errors.New("Add Participant Error")
					}

					self = p
					log.Printf("Join %v ok", self)
				} else if obj.Message.Action == "leave" {
					p = r.(*Room).Get(obj.Message.Display)
					if p == nil {
						return errors.New("Get Participant Error")
					}

					r.(*Room).Remove(p)
					log.Printf("Leave %v ok", p)
				} else {
					p = r.(*Room).Get(obj.Message.Display)
					if p == nil {
						return errors.New("Get Participant Error")
					}
				}

				go r.(*Room).Forward(ctx, p, m)

				return nil
			}

			for m := range inMessages {
				if err := handleMessage(m); err != nil {
					log.Printf("Handle %s err %v", m, err)
					break
				}
			}
		}()

		for m := range outMessages {
			if _, err := c.Write(m); err != nil {
				log.Printf("Websocket Write error %v for %v", err, req.RemoteAddr)
				break
			}
		}
	}))

	http.ListenAndServe(":1988", nil)
}
