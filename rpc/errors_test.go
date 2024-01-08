package rpc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	_ CustomError = &errA{err: nil}
	_ CustomError = &errB{errw: nil}
)

type errA struct {
	err error
}

func (e errA) Error() string {
	return e.err.Error()
}

func (e errA) As(target any) bool {
	return IsMine(e, target)
}

func (e errA) Unwrap() error {
	return e.err
}

func (e errA) Status() int {
	return 500
}

func (e errA) Format(context.Context) any {
	msg := "<nil>"
	if e.err != nil {
		msg = e.err.Error()
	}
	return map[string]string{
		"errA": msg,
	}
}

type errB struct {
	count int
	errw  error
}

func (e errB) Error() string {
	return fmt.Errorf("b: %w", e.errw).Error()
}

func (e errB) As(target any) bool {
	return IsMine(e, target)
}

func (e errB) Unwrap() error {
	return e.errw
}

func (e errB) Status() int {
	return 500
}

func (e errB) Format(context.Context) any {
	fmt.Println("hi from B")
	msg := "<nil>"
	if e.errw != nil {
		msg = e.errw.Error()
	}
	return map[string]string{
		"errB": msg,
	}
}

type errProxy struct {
	ce CustomError
}

func (e errProxy) As(target any) bool {
	fmt.Println("in as")
	return true
}

func (e errProxy) Error() string {
	return e.ce.Error()
}

func IsMine(e CustomError, target any) bool {
	if target, ok := target.(*errProxy); ok {
		mytyp := reflect.TypeOf(e)
		tgttyp := reflect.TypeOf(target.ce)
		fmt.Println("typ", mytyp)
		fmt.Println("tgt", tgttyp)
		if mytyp == tgttyp {
			target.ce = e
			return true
		}
	}
	return false
}

func TestErrors(t *testing.T) {
	// RegisterErrorHandler(&errA{})

	a := errA{err: errors.New("root error")}
	b := errB{errw: a}

	{
		e := errProxy{
			ce: errA{},
		}

		// err val `a` should fit the shape of `errA`
		assert.True(t, errors.As(a, &e), "why")
		val := e.ce.Format(context.Background())
		assert.Equal(t, map[string]string{"errA": "root error"}, val)
		// Code taken from errors.As

		// {
		// 	var errorType = reflect.TypeOf((*error)(nil)).Elem()
		// 	val := reflect.ValueOf(&e)
		// 	typ := val.Type()
		// 	if typ.Kind() != reflect.Ptr || val.IsNil() {
		// 		panic("errors: target must be a non-nil pointer")
		// 	}
		// 	targetType := typ.Elem()
		// 	if targetType.Kind() != reflect.Interface && !targetType.Implements(errorType) {
		// 		panic("errors: *target must be interface or implement error")
		// 	}
		// 	fmt.Println(typ)
		// 	fmt.Println(reflect.TypeOf(a))
		// 	if reflect.TypeOf(a).AssignableTo(targetType) {
		// 		fmt.Println("fuck")
		// 		// val.Elem().Set(reflect.ValueOf(b))
		// 	}
		// }

		f := errProxy{
			ce: errA{},
		}
		val = f.ce.Format(context.Background())
		assert.Equal(t, map[string]string{"errA": "<nil>"}, val)

		// typ := reflect.TypeOf(f)
		// fmt.Println(typ)
		// zeroVal := reflect.Zero(typ).Interface()

		// err val `b` should fit the shape of `errA`, because it wraps `a`
		assert.True(t, errors.As(b, &f))
		val = f.ce.Format(context.Background())
		// this assertion fails because `b` is directly assignable to zeroVal (which is of type `interface{}`)
		// instead it calls Format (above) on `errB`.
		// It should call unwrap on `errB` and then call Format on the unwrapped value instead.
		assert.Equal(t, map[string]string{"errA": "root error"}, val)
	}

	// var e CustomError = errA{}
	// // err val `a` should fit the shape of `errA`
	// assert.True(t, errors.As(a, &e))
	// val := e.Format(context.Background())
	// assert.Equal(t, map[string]string{"errA": "root error"}, val)

	// var f any = errA{}
	// val = f.(CustomError).Format(context.Background())
	// assert.Equal(t, map[string]string{"errA": "<nil>"}, val)

	// typ := reflect.TypeOf(f)
	// fmt.Println(typ)
	// zeroVal := reflect.Zero(typ).Interface()

	// // err val `b` should fit the shape of `errA`, because it wraps `a`
	// assert.True(t, errors.As(b, &zeroVal))
	// val = zeroVal.(CustomError).Format(context.Background())
	// // this assertion fails because `b` is directly assignable to zeroVal (which is of type `interface{}`)
	// // instead it calls Format (above) on `errB`.
	// // It should call unwrap on `errB` and then call Format on the unwrapped value instead.
	// assert.Equal(t, map[string]string{"errA": "root error"}, val)

	// // Code taken from errors.As
	// {
	// 	var errorType = reflect.TypeOf((*error)(nil)).Elem()
	// 	val := reflect.ValueOf(&zeroVal)
	// 	typ := val.Type()
	// 	if typ.Kind() != reflect.Ptr || val.IsNil() {
	// 		panic("errors: target must be a non-nil pointer")
	// 	}
	// 	targetType := typ.Elem()
	// 	if targetType.Kind() != reflect.Interface && !targetType.Implements(errorType) {
	// 		panic("errors: *target must be interface or implement error")
	// 	}
	// 	fmt.Println(typ)
	// 	fmt.Println(targetType)
	// 	fmt.Println(reflect.TypeOf(b))
	// 	if reflect.TypeOf(b).AssignableTo(targetType) {
	// 		fmt.Println("fuck")
	// 		// val.Elem().Set(reflect.ValueOf(b))
	// 	}
	// }

}
