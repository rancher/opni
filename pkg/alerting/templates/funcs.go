package templates

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	amtemplate "github.com/prometheus/alertmanager/template"
	"github.com/prometheus/common/model"
)

func floatToTime(v float64) (*time.Time, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errNaNOrInf
	}
	timestamp := v * 1e9
	if timestamp > math.MaxInt64 || timestamp < math.MinInt64 {
		return nil, fmt.Errorf("%v cannot be represented as a nanoseconds timestamp since it overflows int64", v)
	}
	t := model.TimeFromUnixNano(int64(timestamp)).Time().UTC()
	return &t, nil
}

func convertToFloat(i interface{}) (float64, error) {
	switch v := i.(type) {
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	case int:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("can't convert %T to float", v)
	}
}

var errNaNOrInf = errors.New("value is NaN or Inf")

var DefaultTemplateFuncs = amtemplate.FuncMap{
	"humanize": func(i interface{}) (string, error) {
		v, err := convertToFloat(i)
		if err != nil {
			return "", err
		}
		if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v), nil
		}
		if math.Abs(v) >= 1 {
			prefix := ""
			for _, p := range []string{"k", "M", "G", "T", "P", "E", "Z", "Y"} {
				if math.Abs(v) < 1000 {
					break
				}
				prefix = p
				v /= 1000
			}
			return fmt.Sprintf("%.4g%s", v, prefix), nil
		}
		prefix := ""
		for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
			if math.Abs(v) >= 1 {
				break
			}
			prefix = p
			v *= 1000
		}
		return fmt.Sprintf("%.4g%s", v, prefix), nil
	},
	"humanize1024": func(i interface{}) (string, error) {
		v, err := convertToFloat(i)
		if err != nil {
			return "", err
		}
		if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v), nil
		}
		prefix := ""
		for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
			if math.Abs(v) < 1024 {
				break
			}
			prefix = p
			v /= 1024
		}
		return fmt.Sprintf("%.4g%s", v, prefix), nil
	},
	"humanizeDuration": func(i interface{}) (string, error) {
		v, err := convertToFloat(i)
		if err != nil {
			return "", err
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Sprintf("%.4g", v), nil
		}
		if v == 0 {
			return fmt.Sprintf("%.4gs", v), nil
		}
		if math.Abs(v) >= 1 {
			sign := ""
			if v < 0 {
				sign = "-"
				v = -v
			}
			duration := int64(v)
			seconds := duration % 60
			minutes := (duration / 60) % 60
			hours := (duration / 60 / 60) % 24
			days := duration / 60 / 60 / 24
			// For days to minutes, we display seconds as an integer.
			if days != 0 {
				return fmt.Sprintf("%s%dd %dh %dm %ds", sign, days, hours, minutes, seconds), nil
			}
			if hours != 0 {
				return fmt.Sprintf("%s%dh %dm %ds", sign, hours, minutes, seconds), nil
			}
			if minutes != 0 {
				return fmt.Sprintf("%s%dm %ds", sign, minutes, seconds), nil
			}
			// For seconds, we display 4 significant digits.
			return fmt.Sprintf("%s%.4gs", sign, v), nil
		}
		prefix := ""
		for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
			if math.Abs(v) >= 1 {
				break
			}
			prefix = p
			v *= 1000
		}
		return fmt.Sprintf("%.4g%ss", v, prefix), nil
	},
	"humanizePercentage": func(i interface{}) (string, error) {
		v, err := convertToFloat(i)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%.4g%%", v*100), nil
	},
	"humanizeTimestamp": func(i interface{}) (string, error) {
		v, err := convertToFloat(i)
		if err != nil {
			return "", err
		}

		tm, err := floatToTime(v)
		switch {
		case errors.Is(err, errNaNOrInf):
			return fmt.Sprintf("%.4g", v), nil
		case err != nil:
			return "", err
		}

		return fmt.Sprint(tm), nil
	},
	"formatTime": func(i interface{}) (string, error) {
		ts, ok := i.(time.Time)
		if !ok {
			return "", fmt.Errorf("formatTime: expected time.Time, got %T", i)
		}
		return fmt.Sprint(ts.Format(time.RFC822)), nil
	},
	"toTime": func(i interface{}) (*time.Time, error) {
		v, err := convertToFloat(i)
		if err != nil {
			return nil, err
		}

		return floatToTime(v)
	},
}

func RegisterNewAlertManagerDefaults(amTmplMap, newTmplMap amtemplate.FuncMap) {
	for key := range newTmplMap {
		if err := RegisterTemplateMap(amTmplMap, key, newTmplMap[key]); err != nil {
			panic(err)
		}
	}
}

func RegisterTemplateMap(templateMap amtemplate.FuncMap, key string, templateFunc any) error {
	if _, ok := templateMap[key]; ok {
		return fmt.Errorf("key error : template function %s already exists", key)
	}
	templateMap[key] = templateFunc
	return nil
}
