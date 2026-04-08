// Copyright 2026 Google LLC
package any_test

func foo() {
	var a interface{} // want "use any instead of interface{}"
	_ = a

	var b any
	_ = b

	var c map[string]interface{} // want "use any instead of interface{}"
	_ = c

	var d []interface{} // want "use any instead of interface{}"
	_ = d
}

type I interface { // OK
	Foo()
}

type J interface {
	interface{} // want "use any instead of interface{}"
}

func bar(i interface{}) {} // want "use any instead of interface{}"
