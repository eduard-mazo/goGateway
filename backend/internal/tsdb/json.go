package tsdb

import jsoniter "github.com/json-iterator/go"

// json is a drop-in replacement for encoding/json using jsoniter.
// Reduces reflection overhead and GC pressure at 2000+ signals/sec.
var json = jsoniter.ConfigCompatibleWithStandardLibrary
