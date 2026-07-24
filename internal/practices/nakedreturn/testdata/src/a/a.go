package a

func short() (n int) {
	n = 1
	return
}

func long() (n int, err error) {
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	n = 1
	return // want `naked return in long \(20\+ line body\); name the returned values explicitly`
}
