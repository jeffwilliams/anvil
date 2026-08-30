package anvil

type Window struct {
	Id         int
	GlobalPath string
	Path       string
}

type WindowBody struct {
	Len           int
	WidthInRunes  int
	HeightInRunes int
}

type Notification struct {
	WinId  int
	Op     NotificationOp
	Offset int
	Len    int
	Cmd    []string
}

type Selection struct {
	Start, End, Len int
}

type NotificationOp int

const (
	NotificationOpInsert = iota
	NotificationOpDelete
	NotificationOpExec
	NotificationOpPut
	NotificationOpFileClosed
	NotificationOpFileOpened
	NotificationOpKeyPress
	NotificationOpTextInput
	NotificationOpWinSizeChanged
	NotificationOpWinClosed
)

type ExecuteReq struct {
	Cmd  string
	Args []string
}

type WebsockMessageId int

const (
	WebsockMessageNotification = iota
)

type AnvilInfo struct {
	Cwd     string
	ConfDir string
	Title   string
}

type KeyPress struct {
	KeyName   string
	Modifiers int
}

type Tint struct {
	Start, End int
	Tint       string
}
