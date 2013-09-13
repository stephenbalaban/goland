// Player is a Unit that is controllable by a client
// (this should really have no distinction)
package game

import "encoding/gob"

func init() {
	gob.Register(&Player{})
}

type HP struct {
	current int
	max     int
}

type Player struct {
	*Unit
}

const (
	DEFAULT_HP = 20
)

func NewPlayer(name string) *Player {
	o := NewUnit(name, DEFAULT_HP, DEFAULT_HP)
	o.SetGlyph(GLYPH_HUMAN)
	p := &Player{Unit: o}

	p.SetTag("player", true)
	return p
}
