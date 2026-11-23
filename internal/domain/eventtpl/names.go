package eventtpl

import "strings"

type language struct{ months, days []string }

// Validate takes only these keys of Templates.Names. The days start on
// Sunday, as time.Weekday counts them.
var languages = map[string]language{
	"ru": {
		months: []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"},
		days:   []string{"Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота"},
	},
	"en": {
		months: []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		days:   []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	},
}

func form(field, name string) string {
	switch field {
	case "month":
		return strings.ToLower(name)
	case "mon":
		return string([]rune(name)[:3])
	}
	return name
}
