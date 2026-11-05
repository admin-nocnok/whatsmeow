// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"testing"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestIgnoredNodeMatchesBaileys(t *testing.T) {
	ctx := context.Background()
	own := types.JID{User: "5215500000000", Device: 12, Server: types.DefaultUserServer}
	ownLID := types.JID{User: "100000000000001", Device: 12, Server: types.HiddenUserServer}
	phone, phoneLID := own.ToNonAD(), ownLID.ToNonAD()
	contact := types.NewJID("5215511111111", types.DefaultUserServer)
	group := types.NewJID("120363000000000001", types.GroupServer)
	status := types.StatusBroadcastJID
	cli := &Client{Store: &store.Device{ID: &own, LID: ownLID}}
	var asked types.JID
	cli.ShouldIgnoreJID = func(_ context.Context, chat types.JID) bool {
		asked = chat
		return chat.Server == types.GroupServer || chat.Server == types.BroadcastServer
	}
	node := func(tag string, attrs waBinary.Attrs) *waBinary.Node { return &waBinary.Node{Tag: tag, Attrs: attrs} }
	cases := []struct {
		name    string
		node    *waBinary.Node
		asked   types.JID
		ignored bool
		nack    int
	}{
		{"group message", node("message", waBinary.Attrs{"from": group, "participant": contact}), group, true, NackUnhandledError},
		{"status message", node("message", waBinary.Attrs{"from": status, "participant": contact}), status, true, NackUnhandledError},
		{"direct message", node("message", waBinary.Attrs{"from": contact}), contact, false, 0},
		{"group receipt", node("receipt", waBinary.Attrs{"from": group, "participant": contact}), group, true, 0},
		{"group receipt from our phone", node("receipt", waBinary.Attrs{"from": group, "participant": phone}), group, true, 0},
		{"our receipt for a status", node("receipt", waBinary.Attrs{"from": phone, "recipient": status}), status, true, 0},
		{"our LID receipt for a group", node("receipt", waBinary.Attrs{"from": phoneLID, "recipient": group}), group, true, 0},
		{"our receipt for a contact", node("receipt", waBinary.Attrs{"from": phone, "recipient": contact}), contact, false, 0},
		{"our receipt without recipient", node("receipt", waBinary.Attrs{"from": phone}), types.EmptyJID, false, 0},
		{"status receipt from our LID", node("receipt", waBinary.Attrs{"from": status, "participant": phoneLID}), status, true, 0},
		{"contact receipt", node("receipt", waBinary.Attrs{"from": contact}), contact, false, 0},
		{"group notification", node("notification", waBinary.Attrs{"from": group, "type": "w:gp2"}), group, true, 0},
		{"call from a contact", node("call", waBinary.Attrs{"from": contact}), contact, false, 0},
		{"call from a group", node("call", waBinary.Attrs{"from": group}), group, true, 0},
		{"group chatstate", node("chatstate", waBinary.Attrs{"from": group, "participant": contact}), types.EmptyJID, false, 0},
	}
	for _, c := range cases {
		asked = types.EmptyJID
		ignored, nack := cli.ignoredNode(ctx, c.node)
		if ignored != c.ignored || nack != c.nack || asked != c.asked {
			t.Errorf("%s: got (%v, %d) asking about %v, want (%v, %d) asking about %v", c.name, ignored, nack, asked, c.ignored, c.nack, c.asked)
		}
	}

	cli.ShouldIgnoreJID = func(context.Context, types.JID) bool { return true }
	if ignored, _ := cli.ignoredNode(ctx, node("notification", waBinary.Attrs{"from": types.ServerJID, "type": "encrypt"})); ignored {
		t.Error("the server's own nodes are never dropped")
	}
	cli.ShouldIgnoreJID = nil
	if ignored, _ := cli.ignoredNode(ctx, node("message", waBinary.Attrs{"from": group})); ignored {
		t.Error("without ShouldIgnoreJID nothing is dropped")
	}
}

func TestHandlerQueueSkipsIgnoredNodes(t *testing.T) {
	contact := types.NewJID("5215511111111", types.DefaultUserServer)
	group := types.NewJID("120363000000000001", types.GroupServer)
	handled := make(chan types.JID, 2)
	cli := &Client{Log: waLog.Noop}
	cli.ShouldIgnoreJID = func(_ context.Context, chat types.JID) bool { return chat.Server == types.GroupServer }
	cli.nodeHandlers = map[string]nodeHandler{"message": func(_ context.Context, node *waBinary.Node) {
		handled <- node.AttrGetter().JID("from")
	}}
	queue := make(chan *waBinary.Node, 2)
	queue <- &waBinary.Node{Tag: "message", Attrs: waBinary.Attrs{"id": "1", "from": group, "participant": contact}}
	queue <- &waBinary.Node{Tag: "message", Attrs: waBinary.Attrs{"id": "2", "from": contact}}
	connCtx, cancel := context.WithCancel(context.Background())
	closeWait := make(chan struct{})
	go cli.handlerQueueLoop(context.Background(), connCtx, queue, closeWait)
	select {
	case from := <-handled:
		if from != contact {
			t.Error("the ignored group message reached its handler")
		}
	case <-time.After(5 * time.Second):
		t.Error("the direct message never reached its handler")
	}
	cancel()
	<-closeWait
}
