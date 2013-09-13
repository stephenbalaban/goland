package main

import (
	"github.com/errnoh/termbox/panel"
	"github.com/mischief/gochanio"
	"github.com/mischief/goland/game"
	"github.com/mischief/goland/game/gnet"
	"github.com/mischief/goland/game/gutil"
	"github.com/nsf/termbox-go"
	"image"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
	"time"
)

const (
	FPS_LIMIT = 23
)

var (
	CARDINALS = map[rune]game.Action{
		'w': game.DIR_UP,
		'k': game.DIR_UP,
		'a': game.DIR_LEFT,
		'h': game.DIR_LEFT,
		's': game.DIR_DOWN,
		'j': game.DIR_DOWN,
		'd': game.DIR_RIGHT,
		'l': game.DIR_RIGHT,
		',': game.ACTION_ITEM_PICKUP,
		'x': game.ACTION_ITEM_DROP,
		'i': game.ACTION_ITEM_LIST_INVENTORY,
	}
)

type Game struct {
	player game.Object
	pm     sync.Mutex

	Terminal
	sidepanel  *LogPanel
	logpanel  *LogPanel
	chatpanel *ChatPanel

	panels    map[string]panel.Panel
	mainpanel *panel.Buffered

	CloseChan chan bool

	Objects *game.GameObjectMap
	Map     *game.MapChunk

	config *gutil.LuaConfig

	ServerCon net.Conn

	ServerRChan <-chan interface{}
	ServerWChan chan<- interface{}
}

func NewGame(config *gutil.LuaConfig) *Game {
	g := Game{config: config}
	g.Objects = game.NewGameObjectMap()

	g.CloseChan = make(chan bool, 1)

	g.player = game.NewGameObject("")

	g.mainpanel = panel.MainScreen()
	g.panels = make(map[string]panel.Panel)

	g.panels["stats"] = NewStatsPanel()
	g.panels["view"] = NewViewPanel(&g)
	g.panels["side"] = NewLogPanel(20)
	g.panels["log"] = NewLogPanel(4)
	g.panels["player"] = NewPlayerPanel(&g)
	g.panels["chat"] = NewChatPanel(&g, &g.Terminal)

	g.logpanel = g.panels["log"].(*LogPanel)
	g.sidepanel = g.panels["side"].(*LogPanel)
	g.chatpanel = g.panels["chat"].(*ChatPanel)


	//g.chatbox = NewChatBuffer(&g, &g.Terminal)

	//g.Objects = append(g.Objects, g.Player.GameObject)

	return &g
}

func (g *Game) SendPacket(p *gnet.Packet) {
	log.Printf("Game: SendPacket: %s", p)
	g.ServerWChan <- p
}

func (g *Game) GetPlayer() game.Object {
	g.pm.Lock()
	defer g.pm.Unlock()

	return g.player
}

func (g *Game) Run() {

	g.Start()

	timer := game.NewDeltaTimer()
	ticker := time.NewTicker(time.Second / FPS_LIMIT)

	run := true

	for run {
		select {
		case <-ticker.C:
			// frame tick
			delta := timer.DeltaTime()

			if delta.Seconds() > 0.25 {
				delta = time.Duration(250 * time.Millisecond)
			}

			g.Update(delta)
			g.Draw()

			g.Flush()

		case <-g.CloseChan:
			run = false
		}
	}

	g.End()

}

func (g *Game) Start() {
	log.Print("Game: Starting")

	// network setup
	server, err := g.config.Get("server", reflect.String)
	if err != nil {
		log.Fatal("Game: Start: missing server in config: %s", err)
	}

	con, err := net.Dial("tcp", server.(string))
	if err != nil {
		log.Fatalf("Game: Start: Dial: %s", err)
	}

	g.ServerCon = con

	g.ServerRChan = chanio.NewReader(g.ServerCon)
	g.ServerWChan = chanio.NewWriter(g.ServerCon)

	if g.ServerRChan == nil || g.ServerWChan == nil {
		log.Fatal("Game: Start: can't establish channels")
	}

	// login
	username, err := g.config.Get("username", reflect.String)
	if err != nil {
		log.Fatal("Game: Start: missing username in config: %s", err)
	}

	g.ServerWChan <- gnet.NewPacket("Tconnect", username)

	// request the map from server
	g.ServerWChan <- gnet.NewPacket("Tloadmap", nil)

	// request the object we control
	// XXX: the delay is to fix a bug regarding ordering of packets.
	// if the client gets the response to this before he is notified
	// that the object exists, it will barf, so we delay this request.
	time.AfterFunc(50*time.Millisecond, func() {
		g.ServerWChan <- gnet.NewPacket("Tgetplayer", nil)
	})

	// anonymous function that reads packets from the server
	go func(r <-chan interface{}) {
		for x := range r {
			p, ok := x.(*gnet.Packet)
			if !ok {
				log.Printf("Game: Read: Bogus server packet %#v", x)
				continue
			}

			g.HandlePacket(p)
		}
		log.Println("Game: Read: Disconnected from server!")
		io.WriteString(g.logpanel, "Disconnected from server!")
	}(g.ServerRChan)

	// terminal/keyhandling setup
	g.Terminal.Start()

	// Set the sizes of the logpanel + sidepanel
	w, h := termbox.Size()
	print(w)
	print("sex")
	print(h)
	print("sex2")
	g.logpanel.rect = image.Rect(1, h-7, w/2-1, h-3)
	g.sidepanel.rect = image.Rect(w-1, 3, 3 * w / 4, 18)
	g.sidepanel.lines = 18

	// chat dialog
	//g.TermLog = NewTermLog(image.Pt(g.Terminal.Rect.Width-VIEW_START_X-VIEW_PAD_X, 5))

	// ESC to quit
	g.HandleKey(termbox.KeyEsc, func(ev termbox.Event) { g.CloseChan <- false })

	// Enter to chat
	g.HandleKey(termbox.KeyEnter, func(ev termbox.Event) { g.SetInputHandler(g.chatpanel) })

	// convert to func SetupDirections()
	for k, v := range CARDINALS {
		func(c rune, d game.Action) {
			g.HandleRune(c, func(_ termbox.Event) {
				// lol collision
				p := &gnet.Packet{"Taction", CARDINALS[c]}
				g.SendPacket(p)
				offset := game.DirTable[d]
				g.pm.Lock()
				defer g.pm.Unlock()
				oldposx, oldposy := g.player.GetPos()
				newpos := image.Pt(oldposx+offset.X, oldposy+offset.Y)
				if g.Map.CheckCollision(nil, newpos) {
					g.player.SetPos(newpos.X, newpos.Y)
				}
			})

			/*
				      scale := PLAYER_RUN_SPEED
							upperc := unicode.ToUpper(c)
							g.HandleRune(upperc, func(_ termbox.Event) {
								for i := 0; i < scale; i++ {
									g.Player.Move(d)
								}
							})
			*/
		}(k, v)
	}

}

func (g *Game) End() {
	log.Print("Game: Ending")
	g.Terminal.End()
}

func (g *Game) Update(delta time.Duration) {
	// collect stats

	for _, p := range g.panels {
		if v, ok := p.(InputHandler); ok {
			v.HandleInput(termbox.Event{Type: termbox.EventResize})
		}

		if v, ok := p.(gutil.Updater); ok {
			v.Update(delta)
		}
	}

	g.RunInputHandlers()

	for o := range g.Objects.Chan() {
		o.Update(delta)
	}

}

func (g *Game) Draw() {

	g.Terminal.Clear()
	g.mainpanel.Clear()

	// draw panels
	for _, p := range g.panels {
		if v, ok := p.(panel.Drawer); ok {
			v.Draw()
		}
	}

}

// deal with gnet.Packets received from the server
func (g *Game) HandlePacket(pk *gnet.Packet) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Game: HandlePacket: %s", err)
		}
	}()

	log.Printf("Game: HandlePacket: %s", pk)
	switch pk.Tag {

	// Rside: we got a response to the side panel
	case "Rside":
		g.sidepanel.Reset()
		io.WriteString(g.sidepanel, pk.Data.(string))

	// Rchat: we got a text message
	case "Rchat":
		chatline := pk.Data.(string)
		io.WriteString(g.logpanel, chatline)

	// Raction: something moved on the server
	// Need to update the objects (sync client w/ srv)
	case "Raction":
		robj := pk.Data.(game.Object) // remote object

		for o := range g.Objects.Chan() {
			if o.GetID() == robj.GetID() {
				o.SetPos(robj.GetPos())
			} /*else if o.GetTag("item") {
				item := g.Objects.FindObjectByID(o.GetID())
				if item.GetTag("gettable") {
					item.SetPos(o.GetPos())
				} else {
					g.Objects.RemoveObject(item)
				}
			}	*/
		}

		// Rnewobject: new object we need to track
	case "Rnewobject":
		obj := pk.Data.(game.Object)
		g.Objects.Add(obj)

		// Rdelobject: some object went away
	case "Rdelobject":
		obj := pk.Data.(game.Object)
		g.Objects.RemoveObject(obj)

		// Rgetplayer: find out who we control
	case "Rgetplayer":
		playerid := pk.Data.(int)

		pl := g.Objects.FindObjectByID(playerid)
		if pl != nil {
			g.pm.Lock()
			g.player = pl
			g.pm.Unlock()
		} else {
			log.Printf("Game: HandlePacket: can't find our player %s", playerid)

			// just try again
			// XXX: find a better way
			time.AfterFunc(50*time.Millisecond, func() {
				g.ServerWChan <- gnet.NewPacket("Tgetplayer", nil)
			})
		}

		// Rloadmap: get the map data from the server
	case "Rloadmap":
		gmap := pk.Data.(*game.MapChunk)
		g.Map = gmap

	default:
		log.Printf("bad packet tag %s", pk.Tag)
	}

}
