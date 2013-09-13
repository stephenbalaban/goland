// Unit is a non-static game object
package game

import (
	"fmt"
)

type HP struct {
	current int
	max int
}

type Unit struct {
	Object
	*Inventory

	Level     int
	hp        *HP
}

func NewUnit(name string, hp int, hpmax int) *Unit {
	hps := &HP{ current: hp, max: hpmax }
	u := &Unit{Level: 1, hp: hps }
	u.Inventory = NewInventory()

	ob := NewGameObject(name)
	u.Object = ob

	return u
}

// Checks if a Unit HasItem *Item
func (u Unit) HasItem(i *Item) bool {
	if u.Inventory.ContainsItem(i) {
		return true
	}
	return false
}

func (u Unit) String() string {
	return fmt.Sprintf("%s: Hp: %d(%d) %s", u.GetName(), u.hp.current, u.hp.max, u.Object)
}
