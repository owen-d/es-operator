package util

import (
	"fmt"
)

func MergeMaps(sources ...map[string]string) map[string]string {
	res := make(map[string]string)
	for _, src := range sources {
		for k, v := range src {
			res[k] = v
		}
	}

	return res
}

func ExtractKey(m map[string]string, k string) (res string, err error) {
	if m == nil {
		return res, fmt.Errorf("nil map has no keys")
	}
	if v, ok := m[k]; !ok {
		return res, fmt.Errorf("map has no key: %s", k)
	} else {
		return v, nil
	}
}
