package core

// Wave 9 (docs/EXPERIMENTS.md, "2026-11-15 wave 9"): App details' Add to
// Channel under flexpane_content_parity (build c07ab895506f).
//
// With the experiment on, App details (gantry-v2-async-client-app-details)
// lays its buttons out from getAppPrimaryActions (boot-shared 8oqt TW) over
// useAppPrimaryActionFacts (3QxE O, called with requiresInvitableChannel:
// flexpane_content_parity), and its Add to channel, both the main button
// and AppDetailsOverflow's item (onAddToChannel when the experiment is
// on), opens openAddAgentToChannelsDialog (gWR0 w: AddAgentToChannelsModal).
// Off, the overflow's item opens openInviteMemberToChannelDialog (the
// single-channel dialog) and no primary action is Add to Channel.
//
// This file is the data half: bD9l's invitable channels and the modal's
// submit (aggregateInviteResults).

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/some-du6e/clack/internal/slack"
)

// FlexpaneContentParity is HLQ9 X7: flexpane_content_parity is on.
func (a *App) FlexpaneContentParity() bool { return a.Experiment(ExpFlexpaneContentParity) == "on" }

// canInviteAppToChannel is bD9l canInviteAppToChannel(state, channel,
// member): a public channel the viewer is in, or a private one they're in
// (JUrK cV), not another team's (Opi6 lo: only the viewer's own
// conversations are in the store's Convs), one the viewer can post in
// (o/IU c: not archived, not read-only or thread-only, as
// channel_managers.go reads it), and not a Slack Connect channel when the
// member is an Agentforce or Sidekick bot (5DYq ju, cfd/ D, 7jev o).
func (a *App) canInviteAppToChannel(c *slack.Conversation, mem *slack.User) bool {
	if c == nil || c.IsIM || c.IsMPIM {
		return false
	}
	if !a.isMemberConv(c) || c.IsArchived || !a.PostingFor(c).CanPost() {
		return false
	}
	if c.IsExtShared && mem != nil && (mem.IsAgentforceBot || mem.IsSidekickBot) {
		return false
	}
	return true
}

// AppInvitableChannels is bD9l dX: the conversations the app's bot user
// member can be added to, sorted by name (AddAgentToChannelsModal's
// options: sort(LF("name"))).
func (a *App) AppInvitableChannels(member string) []Conv {
	mem, _ := a.S.User(member)
	var out []Conv
	for _, c := range a.storeConvs() {
		if a.canInviteAppToChannel(c, mem) {
			out = append(out, a.conv(c))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		x, y := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if x != y {
			return x < y
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// HasAppInvitableChannel is bD9l U3: some conversation takes the member.
func (a *App) HasAppInvitableChannel(member string) bool {
	mem, _ := a.S.User(member)
	for _, c := range a.storeConvs() {
		if a.canInviteAppToChannel(c, mem) {
			return true
		}
	}
	return false
}

// storeConvs copies the store's conversations (the viewer's own).
func (a *App) storeConvs() []*slack.Conversation {
	a.S.RLock()
	defer a.S.RUnlock()
	cs := make([]*slack.Conversation, 0, len(a.S.Convs))
	for _, c := range a.S.Convs {
		cs = append(cs, c)
	}
	return cs
}

// CanInviteAppTo is bD9l Vg for conversation ch (canInviteToCurrentChannel).
func (a *App) CanInviteAppTo(ch, member string) bool {
	c, ok := a.S.Conv(ch)
	if !ok {
		return false
	}
	mem, _ := a.S.User(member)
	return a.canInviteAppToChannel(c, mem)
}

// The reasons a channel failed (gWR0 E over conversations.invite's errors,
// pU9y ky); every other error is "other".
const (
	AddToChannelNotInWorkspace = "notInWorkspace"
	AddToChannelNotAllowed     = "notAllowed"
	AddToChannelNoPermission   = "noPermission"
	AddToChannelOther          = "other"
)

var addToChannelReasons = map[string]string{
	"org_user_not_in_team":                      AddToChannelNotInWorkspace,
	"invitee_is_not_a_member_of_this_workspace": AddToChannelNotInWorkspace,
	"invitee_cant_see_channel":                  AddToChannelNotInWorkspace,
	"cant_invite_agent":                         AddToChannelNotAllowed,
	"cant_invite_app_user":                      AddToChannelNotAllowed,
	"app_not_available_in_channel":              AddToChannelNotAllowed,
	"app_access_restricted":                     AddToChannelNotAllowed,
	"app_cannot_join_channel":                   AddToChannelNotAllowed,
	"no_permission":                             AddToChannelNoPermission,
	"cant_invite_to_managed_channel":            AddToChannelNoPermission,
	"invalid_for_restrict_guests_channel":       AddToChannelNoPermission,
}

// AddToChannelFailure is one channel the member couldn't be added to.
type AddToChannelFailure struct{ Channel, Reason string }

// AddToChannelsResult is aggregateInviteResults: the channels the member
// joined, those it was already in (already_in_channel, cant_invite_self)
// and the failures, each in the order asked.
type AddToChannelsResult struct {
	NewlyAdded, AlreadyMember []string
	Failed                    []AddToChannelFailure
}

// inviteErrorCode is getInviteErrorCode: data.errors[0].error, else the
// error's name.
func inviteErrorCode(err error) string {
	var se *slack.Error
	if !errors.As(err, &se) {
		return ""
	}
	var r struct {
		Errors []struct {
			Error string `json:"error"`
		} `json:"errors"`
	}
	if len(se.Body) > 0 && json.Unmarshal(se.Body, &r) == nil && len(r.Errors) > 0 && r.Errors[0].Error != "" {
		return r.Errors[0].Error
	}
	return se.Code
}

// AddMemberToChannels is AddAgentToChannelsModal's submit: one
// conversations.invite per channel (submitInviteToChannel with reason
// add-agent-to-channels-modal; Promise.allSettled, so every channel is
// asked), then aggregateInviteResults.
func (a *App) AddMemberToChannels(ctx context.Context, member string, chans []string) AddToChannelsResult {
	errs := make([]error, len(chans))
	done := make(chan struct{}, len(chans))
	for i, ch := range chans {
		go func() {
			errs[i] = a.Invite(ctx, ch, member)
			done <- struct{}{}
		}()
	}
	for range chans {
		<-done
	}
	var r AddToChannelsResult
	for i, ch := range chans {
		if errs[i] == nil {
			r.NewlyAdded = append(r.NewlyAdded, ch)
			continue
		}
		code := inviteErrorCode(errs[i])
		if code == "already_in_channel" || code == "cant_invite_self" {
			r.AlreadyMember = append(r.AlreadyMember, ch)
			continue
		}
		reason, ok := addToChannelReasons[code]
		if !ok {
			reason = AddToChannelOther
		}
		r.Failed = append(r.Failed, AddToChannelFailure{Channel: ch, Reason: reason})
	}
	return r
}
