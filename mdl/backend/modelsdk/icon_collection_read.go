// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// ListIconCollections reads every CustomIcons$CustomIconCollection unit (identity,
// prefix, export level, and the named icons with their character codes) for
// SHOW / DESCRIBE ICON COLLECTION. Icon collections are read-only in mxcli — they
// ship with the theme/Atlas and are referenced from widgets as
// `Module.CollectionName.IconName` (e.g. a button's `icon:` property).
func (b *Backend) ListIconCollections() ([]*types.IconCollection, error) {
	units, err := b.reader.ListRawUnitsByType("CustomIcons$CustomIconCollection")
	if err != nil {
		return nil, err
	}
	out := make([]*types.IconCollection, 0, len(units))
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal icon collection %s: %w", u.ID, err)
		}
		ic := &types.IconCollection{ContainerID: model.ID(u.ContainerID)}
		ic.ID = model.ID(u.ID)
		ic.TypeName = "CustomIcons$CustomIconCollection"
		ic.Name, _ = doc["Name"].(string)
		ic.Prefix, _ = doc["Prefix"].(string)
		ic.Documentation, _ = doc["Documentation"].(string)
		ic.ExportLevel, _ = doc["ExportLevel"].(string)
		// The Icons array carries a leading int marker (like other Mendix lists);
		// skip any non-document element.
		if arr, ok := doc["Icons"].(bson.A); ok {
			for _, el := range arr {
				iconDoc, ok := el.(bson.M)
				if !ok {
					continue
				}
				item := types.IconItem{}
				item.Name, _ = iconDoc["Name"].(string)
				switch cc := iconDoc["CharacterCode"].(type) {
				case int32:
					item.CharacterCode = int(cc)
				case int64:
					item.CharacterCode = int(cc)
				}
				if tags, ok := iconDoc["Tags"].(bson.A); ok {
					for _, t := range tags {
						if s, ok := t.(string); ok {
							item.Tags = append(item.Tags, s)
						}
					}
				}
				ic.Icons = append(ic.Icons, item)
			}
		}
		out = append(out, ic)
	}
	return out, nil
}
