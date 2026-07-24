package main

func helper() {
	panic("boom") // want `panic\(\) in package main`
}

func main() {
	helper()
}
