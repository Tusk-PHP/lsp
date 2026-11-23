package laravel

import "strings"

const notificationClass = `Illuminate\Notifications\Notification`

// notificationMethods are the methods Laravel's notification channels and
// sender call on a notification by convention: Notification declares none of
// them, each notification class writes the ones its channels need.
var notificationMethods = map[string]bool{
	"via":                        true,
	"viaconnections":             true,
	"viaqueues":                  true,
	"shouldsend":                 true,
	"withdelay":                  true,
	"middleware":                 true,
	"aftersending":               true,
	"toarray":                    true,
	"tomail":                     true,
	"todatabase":                 true,
	"databasetype":               true,
	"initialdatabasereadatvalue": true,
	"tobroadcast":                true,
	"broadcasttype":              true,
	"tovonage":                   true,
	"tonexmo":                    true,
	"toslack":                    true,
	"deduplicationid":            true,
	"withdeduplicators":          true,
	"messagegroup":               true,
	"withmessagegroups":          true,
}

// ConventionMethod reports whether Laravel calls method on an instance of
// class by convention, without the class declaring it. class is one class of
// the receiver's hierarchy, fully qualified, and method names ignore case.
func ConventionMethod(class, method string) bool {
	if strings.EqualFold(strings.TrimPrefix(class, `\`), notificationClass) {
		return notificationMethods[strings.ToLower(method)]
	}
	return false
}
