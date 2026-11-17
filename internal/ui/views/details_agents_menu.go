package views

// The reworked Agents & apps roster's member actions (ia-details-popover-
// members chunk, build c07ab895506f; core/exp_wave10.go has the checks):
//
//   - A row of the channel (IADetailsPopoverMemberItemComponent with
//     isInChannel, not Slackbot) gets its secondary content (…
//     __members_list_item_secondary_content: margin-right 24px, 28px tall,
//     centred), whose IaDetailsPopoverMemberItemActionsMenu is a 28px
//     outline icon c-button (c-button--icon c-button--medium,
//     "vertical-ellipsis" at 16px, aria-label "More options") at the row's
//     right edge less 16px (…__members_list_item_action_menu: position
//     absolute, right 16px), shown while the row is hovered or its menu is
//     open (visibility hidden otherwise).
//   - Its menu (MenuTrigger position "bottom-right", the 300px c-menu) has,
//     in this order, Remove from Channel Managers, Make Channel Manager,
//     Edit agent settings and the danger "Remove from channel"; with none of
//     them the button isn't drawn. agent_channel_header (M/qt Zc) hides the
//     two Channel Manager items for an app, a bot or the native channel
//     expert, and offers Edit agent settings only for the native and the
//     Agentforce channel experts (canEditAgent: Zc, in the channel, the
//     viewer can manage the channel, the configuration not locked). Clack
//     draws neither expert's row, and doesn't port the Channel Manager
//     items (admin.roles writes and MakeChannelManagerModal); for the agents
//     and apps it lists the menu is "Remove from channel" alone, where
//     core.App.CanRemoveMemberFromChannel (tu's canRemoveMember) allows it.
//     With Zc off Slack would also offer Make Channel Manager on these rows
//     to a viewer who may edit the channel's managers; that half is the
//     unported item.
//   - Remove from channel is openRemoveMemberDialog (popups/remove_member.go);
//     once the kick is done (onMemberRemoved) the row leaves the roster and
//     the counts load again.

import (
	"context"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/ui/popups"
)

// cdRowMenuBtn is the menu button's size (c-button--icon.c-button--medium)
// and cdRowMenuRight its inset from the row's right edge.
const (
	cdRowMenuBtn   = 28
	cdRowMenuRight = 16
	// cdRowSecondaryMargin is the secondary content's margin-right.
	cdRowSecondaryMargin = 24
)

// cdRowMenu is one row's "…" and its menu.
type cdRowMenu struct {
	btn    widget.Clickable
	press  int         // the button's press tracker tag
	local  image.Point // the last press in the button's coordinates
	size   image.Point // the button as last drawn
	open   bool        // the menu is showing (…_action_menu--visible)
	menu   *popups.Menu
	anchor image.Rectangle // the button in window coordinates when it opened
}

func (d *ChannelDetails) rowMenu(id string) *cdRowMenu {
	ag := d.ag()
	if ag.rowMenus == nil {
		ag.rowMenus = map[string]*cdRowMenu{}
	}
	m := ag.rowMenus[id]
	if m == nil {
		m = &cdRowMenu{}
		ag.rowMenus[id] = m
	}
	return m
}

// agentRowMenuItems are the row's menu items for member u (the actions
// menu's renderMenu).
func (d *ChannelDetails) agentRowMenuItems(u core.User) []popups.MenuItem {
	c := d.h.Core()
	if c == nil || !c.CanRemoveMemberFromChannel(d.id, u.ID) {
		return nil
	}
	id := u.ID
	return []popups.MenuItem{{Label: nsChannelDetails.T("Remove from channel"), Danger: true, Do: func() { d.openRemoveMember(id) }}}
}

// openRemoveMember is the item's onSelected (N: openRemoveMemberDialog).
func (d *ChannelDetails) openRemoveMember(id string) {
	ph, ok := d.h.(popups.Host)
	if !ok {
		return
	}
	popups.OpenRemoveMemberDialog(ph, d.id, id, func() { d.agentRemoved(id) })
}

// agentRemoved is onMemberRemoved: the row goes and the counts load again.
func (d *ChannelDetails) agentRemoved(id string) {
	ag := d.ag()
	for i, u := range ag.added {
		if u.ID == id {
			ag.added = append(ag.added[:i:i], ag.added[i+1:]...)
			break
		}
	}
	c, ch := d.h.Core(), d.id
	d.loadAgentRoster("")
	d.counts.load(d.h, true, func(ctx context.Context) (core.MemberCounts, error) { return c.MemberCounts(ctx, ch) })
	d.h.Invalidate()
}

// openRowMenu opens the row's menu under its button (bottom-right).
func (d *ChannelDetails) openRowMenu(m *cdRowMenu, items []popups.MenuItem) {
	ph, ok := d.h.(popups.Host)
	if !ok || len(items) == 0 {
		return
	}
	at := d.press.at.Sub(m.local) // the button's window origin
	anchor := image.Rectangle{Min: at, Max: at.Add(m.size)}
	m.open, m.anchor = true, anchor
	m.menu = popups.NewMenuOpts(ph, items, popups.MenuOpts{Anchor: anchor, Popover: "bottom-right",
		OnClose: func() { m.open = false; d.h.Invalidate() }})
	ph.ShowOverlay(m.menu)
}

// rowMenuButton draws the row's "…" at its place in a row size wide and
// tall, when the row is hovered (hover) or the menu is open.
func (d *ChannelDetails) rowMenuButton(gtx C, m *cdRowMenu, size image.Point, hover bool) {
	if !hover && !m.open {
		return
	}
	s := gtx.Dp(cdRowMenuBtn)
	pos := image.Pt(size.X-gtx.Dp(cdRowMenuRight)-s, (size.Y-s)/2)
	defer op.Offset(pos).Push(gtx.Ops).Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &m.press, Kinds: pointer.Press})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			m.local = e.Position.Round()
		}
	}
	g := gtx
	g.Constraints = layout.Exact(image.Pt(s, s))
	m.btn.Layout(g, func(gtx C) D {
		pointerCursor(gtx)
		r := image.Rect(0, 0, s, s)
		rad := gtx.Dp(8)
		rrect(gtx, r, rad, d.p.Bg)
		if m.btn.Hovered() || m.open {
			rrect(gtx, r, rad, d.p.Hover)
		}
		border(gtx, r, rad, max(1, gtx.Dp(1)), d.p.BorderStr)
		layout.Center.Layout(gtx, func(gtx C) D { return d.icon(gtx, "vertical-ellipsis", 16, d.p.Text) })
		return D{Size: image.Pt(s, s)}
	})
	area := clip.Rect{Max: image.Pt(s, s)}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &m.press)
	pass.Pop()
	area.Pop()
	m.size = image.Pt(s, s)
}

// AgentRowMenuItems are the labels of the row menu for member id, as the
// row would offer them (tests; nil: no button).
func (d *ChannelDetails) AgentRowMenuItems(id string) []string {
	u, ok := d.h.Core().User(id)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range d.agentRowMenuItems(u) {
		out = append(out, it.Label)
	}
	return out
}

// OpenAgentRowMenu opens the row menu for member id as a click on its
// button does and returns it (tests; nil when the row has none).
func (d *ChannelDetails) OpenAgentRowMenu(id string) *popups.Menu {
	u, ok := d.h.Core().User(id)
	if !ok {
		return nil
	}
	items := d.agentRowMenuItems(u)
	if len(items) == 0 {
		return nil
	}
	m := d.rowMenu(id)
	d.openRowMenu(m, items)
	return m.menu
}

// AgentRowClickables are member id's row and its "…" button (tests).
func (d *ChannelDetails) AgentRowClickables(id string) (row, menu *widget.Clickable) {
	return dirClick(d.memberClk, "ag:"+id), &d.rowMenu(id).btn
}

// AgentRowMenuAnchor is the "…" in window coordinates when its menu last
// opened (tests).
func (d *ChannelDetails) AgentRowMenuAnchor(id string) image.Rectangle { return d.rowMenu(id).anchor }
