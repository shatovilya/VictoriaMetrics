// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package notifier

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	htmlTpl "html/template"
	textTpl "text/template"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmalert/datasource"
)

// QueryFn is used to wrap a call to datasource into simple-to-use function
// for templating functions.
type QueryFn func(query string) ([]datasource.Metric, error)

func funcsWithQuery(query QueryFn) textTpl.FuncMap {
	fm := make(textTpl.FuncMap)
	for k, fn := range tmplFunc {
		fm[k] = fn
	}
	fm["query"] = func(q string) ([]datasource.Metric, error) {
		return query(q)
	}
	return fm
}

var tmplFunc textTpl.FuncMap

// InitTemplateFunc initiates template helper functions
func InitTemplateFunc(externalURL *url.URL) {
	tmplFunc = textTpl.FuncMap{
		"args": func(args ...interface{}) map[string]interface{} {
			result := make(map[string]interface{})
			for i, a := range args {
				result[fmt.Sprintf("arg%d", i)] = a
			}
			return result
		},
		"reReplaceAll": func(pattern, repl, text string) string {
			re := regexp.MustCompile(pattern)
			return re.ReplaceAllString(text, repl)
		},
		"safeHtml": func(text string) htmlTpl.HTML {
			return htmlTpl.HTML(text)
		},
		"match":   regexp.MatchString,
		"title":   strings.Title,
		"toUpper": strings.ToUpper,
		"toLower": strings.ToLower,
		"humanize": func(v float64) string {
			if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Sprintf("%.4g", v)
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
				return fmt.Sprintf("%.4g%s", v, prefix)
			}
			prefix := ""
			for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
				if math.Abs(v) >= 1 {
					break
				}
				prefix = p
				v *= 1000
			}
			return fmt.Sprintf("%.4g%s", v, prefix)
		},
		"humanize1024": func(v float64) string {
			if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Sprintf("%.4g", v)
			}
			prefix := ""
			for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
				if math.Abs(v) < 1024 {
					break
				}
				prefix = p
				v /= 1024
			}
			return fmt.Sprintf("%.4g%s", v, prefix)
		},
		"humanizeDuration": func(v float64) string {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Sprintf("%.4g", v)
			}
			if v == 0 {
				return fmt.Sprintf("%.4gs", v)
			}
			if math.Abs(v) >= 1 {
				sign := ""
				if v < 0 {
					sign = "-"
					v = -v
				}
				seconds := int64(v) % 60
				minutes := (int64(v) / 60) % 60
				hours := (int64(v) / 60 / 60) % 24
				days := int64(v) / 60 / 60 / 24
				// For days to minutes, we display seconds as an integer.
				if days != 0 {
					return fmt.Sprintf("%s%dd %dh %dm %ds", sign, days, hours, minutes, seconds)
				}
				if hours != 0 {
					return fmt.Sprintf("%s%dh %dm %ds", sign, hours, minutes, seconds)
				}
				if minutes != 0 {
					return fmt.Sprintf("%s%dm %ds", sign, minutes, seconds)
				}
				// For seconds, we display 4 significant digits.
				return fmt.Sprintf("%s%.4gs", sign, v)
			}
			prefix := ""
			for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
				if math.Abs(v) >= 1 {
					break
				}
				prefix = p
				v *= 1000
			}
			return fmt.Sprintf("%.4g%ss", v, prefix)
		},
		"humanizePercentage": func(v float64) string {
			return fmt.Sprintf("%.4g%%", v*100)
		},
		"humanizeTimestamp": func(v float64) string {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Sprintf("%.4g", v)
			}
			t := TimeFromUnixNano(int64(v * 1e9)).Time().UTC()
			return fmt.Sprint(t)
		},
		"pathPrefix": func() string {
			return externalURL.Path
		},
		"externalURL": func() string {
			return externalURL.String()
		},
		"pathEscape": func(u string) string {
			return url.PathEscape(u)
		},
		"queryEscape": func(q string) string {
			return url.QueryEscape(q)
		},
		"crlfEscape": func(q string) string {
			q = strings.Replace(q, "\n", `\n`, -1)
			return strings.Replace(q, "\r", `\r`, -1)
		},
		"quotesEscape": func(q string) string {
			return strings.Replace(q, `"`, `\"`, -1)
		},
		// query function supposed to be substituted at funcsWithQuery().
		// it is present here only for validation purposes, when there is no
		// provided datasource.
		"query": func(q string) ([]datasource.Metric, error) {
			return nil, nil
		},
		"first": func(metrics []datasource.Metric) (datasource.Metric, error) {
			if len(metrics) > 0 {
				return metrics[0], nil
			}
			return datasource.Metric{}, errors.New("first() called on vector with no elements")
		},
		"label": func(label string, m datasource.Metric) string {
			return m.Label(label)
		},
		"value": func(m datasource.Metric) float64 {
			return m.Value
		},
	}
}

// Time is the number of milliseconds since the epoch
// (1970-01-01 00:00 UTC) excluding leap seconds.
type Time int64

// TimeFromUnixNano returns the Time equivalent to the Unix Time
// t provided in nanoseconds.
func TimeFromUnixNano(t int64) Time {
	return Time(t / nanosPerTick)
}

// The number of nanoseconds per minimum tick.
const nanosPerTick = int64(minimumTick / time.Nanosecond)

// MinimumTick is the minimum supported time resolution. This has to be
// at least time.Second in order for the code below to work.
const minimumTick = time.Millisecond

// second is the Time duration equivalent to one second.
const second = int64(time.Second / minimumTick)

// Time returns the time.Time representation of t.
func (t Time) Time() time.Time {
	return time.Unix(int64(t)/second, (int64(t)%second)*nanosPerTick)
}
