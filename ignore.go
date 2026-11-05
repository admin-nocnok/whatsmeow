// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// ignoredNode is Baileys' processNode fast path: whether ShouldIgnoreJID drops node, and the error code of its ack.
func (cli *Client) ignoredNode(ctx context.Context, node *waBinary.Node) (bool, int) {
	if cli.ShouldIgnoreJID == nil {
		return false, 0
	}
	switch node.Tag {
	case "message", "receipt", "notification", "call":
	default:
		return false, 0
	}
	ag := node.AttrGetter()
	chat := ag.OptionalJIDOrEmpty("from")
	if node.Tag == "receipt" && chat.Server != types.GroupServer {
		// A receipt from one of our own devices names the chat in recipient.
		sender := ag.OptionalJIDOrEmpty("participant")
		if sender.IsEmpty() {
			sender = chat
		}
		own := cli.getOwnID()
		if chat.Server == types.HiddenUserServer || chat.Server == types.HostedLIDServer {
			own = cli.getOwnLID()
		}
		if sender.User != "" && sender.User == own.User {
			chat = ag.OptionalJIDOrEmpty("recipient")
		}
	}
	if chat.IsEmpty() || chat == types.ServerJID || !cli.ShouldIgnoreJID(ctx, chat) {
		return false, 0
	}
	if node.Tag == "message" {
		return true, NackUnhandledError
	}
	return true, 0
}
