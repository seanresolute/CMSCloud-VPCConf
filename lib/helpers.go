package lib

import (
	"reflect"
)

func handleSlice(input reflect.Value) []interface{} {
	sliceValue := []interface{}{}
	for i := 0; i < input.Len(); i++ {
		v := input.Index(i)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		if v.Kind() == reflect.Struct {
			sliceValue = append(sliceValue, ObjectToMap(v.Interface()))
		} else {
			sliceValue = append(sliceValue, v.Interface())
		}
	}
	return sliceValue
}

func handleMap(input reflect.Value) map[string]interface{} {
	mapValue := map[string]interface{}{}
	for _, key := range input.MapKeys() {
		v := input.MapIndex(key)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		switch mapValueKind := v.Kind(); mapValueKind {
		case reflect.Struct:
			mapValue[key.String()] = ObjectToMap(v.Interface())
		case reflect.Slice:
			mapValue[key.String()] = handleSlice(v)
		default:
			mapValue[key.String()] = v.Interface()
		}
	}
	return mapValue
}

func ObjectToMap(input interface{}) map[string]interface{} {
	output := map[string]interface{}{}
	if input == nil {
		return output
	}

	v := reflect.TypeOf(input)
	reflectValue := reflect.Indirect(reflect.ValueOf(input))
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		output[v.Kind().String()] = reflectValue.Interface()
		return output
	}

	for i := 0; i < v.NumField(); i++ {
		fieldName := v.Field(i).Name
		field := reflectValue.Field(i)
		kind := field.Kind()
		if kind == reflect.Ptr {
			if field.IsNil() {
				output[fieldName] = map[string]interface{}{}
				continue
			} else {
				field = reflect.Indirect(field)
				kind = field.Kind()
			}
		}
		switch kind {
		case reflect.Struct:
			output[fieldName] = ObjectToMap(field.Interface())
		case reflect.Slice:
			output[fieldName] = handleSlice(field)
		case reflect.Map:
			output[fieldName] = handleMap(field)
		case reflect.String:
			output[fieldName] = field.String()
		default:
			output[fieldName] = field.Interface()
		}
	}
	return output
}
