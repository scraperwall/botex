package matchers

import "regexp"

var (
	Assets *regexp.Regexp
)

func init() {
	Assets = regexp.MustCompile(`\.(jpe?g|png|gif|webp|tiff?|pdf|css|js|woff2?|ttf|eot|svg|ttc)\b`)
}
