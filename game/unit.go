// Unit is a non-static game object
package game

import (
	"fmt"
)

type Unit struct {
	Object
	*Inventory

	Level     int
	Hp, HpMax int
}

func NewUnit(name string, hp int, hpmax int) *Unit {
	u := &Unit{Level: 1,
		Hp:    hp,
		HpMax: hpmax,
	}
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
	return fmt.Sprintf("%s: Hp: %d(%d) %s", u.GetName(), u.Hp, u.HpMax, u.Object)
}
