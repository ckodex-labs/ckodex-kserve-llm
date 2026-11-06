package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// unmarshalUniqueJSON permits unknown fields but rejects ambiguous object keys
// at every depth, including fields not represented in the destination type.
func unmarshalUniqueJSON(data []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := readUniqueJSONValue(decoder, 0, reflect.TypeOf(dest)); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return json.Unmarshal(data, dest)
}

func readUniqueJSONValue(decoder *json.Decoder, depth int, typ reflect.Type) error {
	if depth > 10000 {
		return fmt.Errorf("JSON nesting exceeds 10000 levels")
	}
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		keys := make(map[string]struct{})
		fields := make(map[int]string)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid JSON object key")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			keys[key] = struct{}{}
			fieldType, fieldIndex := jsonFieldType(typ, key)
			if fieldIndex >= 0 {
				if previous, exists := fields[fieldIndex]; exists {
					return fmt.Errorf("JSON keys %q and %q target the same field", previous, key)
				}
				fields[fieldIndex] = key
			}
			if err := readUniqueJSONValue(decoder, depth+1, fieldType); err != nil {
				return err
			}
		}
	case '[':
		var elementType reflect.Type
		if typ != nil && (typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array) {
			elementType = typ.Elem()
		}
		for decoder.More() {
			if err := readUniqueJSONValue(decoder, depth+1, elementType); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	_, err = decoder.Token()
	return err
}

// jsonFieldType mirrors field matching for the non-embedded structs decoded by
// this command. Maps remain case-sensitive, as required for Kubernetes metadata.
func jsonFieldType(typ reflect.Type, key string) (reflect.Type, int) {
	if typ == nil {
		return nil, -1
	}
	if typ.Kind() == reflect.Map {
		return typ.Elem(), -1
	}
	if typ.Kind() != reflect.Struct {
		return nil, -1
	}
	caseMatch := -1
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		if key == name {
			return field.Type, i
		}
		if caseMatch < 0 && strings.EqualFold(key, name) {
			caseMatch = i
		}
	}
	if caseMatch >= 0 {
		return typ.Field(caseMatch).Type, caseMatch
	}
	return nil, -1
}
