package tg

import (
	"encoding/json"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/tidwall/gjson"
)

func getTitle(source any) string {
	var name string
	switch u := source.(type) {
	case *tg.User:
		name = u.FirstName
		if u.LastName != "" {
			name += " " + u.LastName
		}

	case *tg.Chat:
		name = u.Title
	case *tg.Channel:
		name = u.Title
	}

	return name
}

func getUsername(source any) string {
	var username string
	switch u := source.(type) {
	case *tg.User:
		username = u.Username
		if username == "" {
			username = peerHandle(u)
		}
	case *tg.Chat:
		username = fmt.Sprintf("cht[%d]", u.ID)
	case *tg.Channel:
		username = u.Username
		if username == "" {
			username = fmt.Sprintf("chn[%d:%d]", u.ID, u.AccessHash)
		}
	}

	return username
}

// peerHandle returns the id-based handle: usr[id:hash], cht[id], chn[id:hash].
func peerHandle(source any) string {
	switch u := source.(type) {
	case *tg.User:
		return fmt.Sprintf("usr[%d:%d]", u.ID, u.AccessHash)
	case *tg.Chat:
		return fmt.Sprintf("cht[%d]", u.ID)
	case *tg.Channel:
		return fmt.Sprintf("chn[%d:%d]", u.ID, u.AccessHash)
	}
	return ""
}

// cleanJSON removes empty/default fields from JSON
func cleanJSON(data []byte) []byte {
	result := gjson.ParseBytes(data)
	cleaned := cleanValue(result)
	if cleaned == nil {
		return data // Return original if cleaning failed
	}

	cleanedJSON, err := json.Marshal(cleaned)
	if err != nil {
		return data // Return original if marshaling failed
	}

	return cleanedJSON
}

func cleanValue(v gjson.Result) interface{} {
	switch v.Type {
	case gjson.String:
		if v.String() == "" {
			return nil
		}
		return v.String()
	case gjson.Number:
		// return nil
		if v.Int() == 0 && v.Float() == 0 {
			return nil
		}
		return v.Value()
	case gjson.True:
		return nil
		// return true
	case gjson.False:
		return nil
	case gjson.Null:
		return nil
	case gjson.JSON:
		if v.IsArray() {
			arr := make([]interface{}, 0)
			v.ForEach(func(_, item gjson.Result) bool {
				if cleaned := cleanValue(item); cleaned != nil {
					arr = append(arr, cleaned)
				}
				return true
			})
			if len(arr) == 0 {
				return nil
			}
			return arr
		}
		if v.IsObject() {
			obj := make(map[string]interface{})
			v.ForEach(func(key, val gjson.Result) bool {
				if cleaned := cleanValue(val); cleaned != nil {
					obj[key.String()] = cleaned
				}
				return true
			})
			if len(obj) == 0 {
				return nil
			}
			return obj
		}
	}
	return nil
}
