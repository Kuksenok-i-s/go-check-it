package a

func do_thing() {} // want `do_thing uses underscores; Go names use MixedCaps, not snake_case`

func doThing() {}

type my_type struct{} // want `my_type uses underscores; Go names use MixedCaps, not snake_case`
