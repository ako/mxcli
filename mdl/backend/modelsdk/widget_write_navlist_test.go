// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestNavListItemToGen_WritesNames guards ledger finding #24: the modelsdk
// writer must emit the navigation item's Name and give the caption's generated
// DynamicText a name — otherwise Studio Pro rejects the project with CE7247
// "name cannot be empty" / CE0495 "duplicate name ”".
func TestNavListItemToGen_WritesNames(t *testing.T) {
	item := &pages.NavigationListItem{
		Name: "itemTransactions",
		Caption: &model.Text{
			Translations: map[string]string{"en_US": "Transactions"},
		},
	}
	el, err := navListItemToGen(item)
	if err != nil {
		t.Fatalf("navListItemToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(raw, []byte("itemTransactions")) {
		t.Error("encoded item is missing its Name (CE7247/CE0495 in Studio Pro)")
	}
	if !bytes.Contains(raw, []byte("text_itemTransactions")) {
		t.Error("caption DynamicText is missing its name (empty-name error in Studio Pro)")
	}
}
