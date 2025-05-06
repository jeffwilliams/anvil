package expr

var Debug func(message string, args ...interface{})

func dbg(message string, args ...interface{}) {
	if Debug == nil {
		return
	}
	Debug(message, args...)
}
