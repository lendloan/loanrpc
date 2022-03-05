package service

import (
	"os"
	"strconv"
)

type CmdArgs struct {
	args map[string]interface{}
}

var gCmdArgs *CmdArgs

func GetCmdArgs() *CmdArgs {
	if nil == gCmdArgs {
		gCmdArgs = &CmdArgs{
			args: make(map[string]interface{}),
		}

		arg_size := len(os.Args[1:])
		for i := 1; i < arg_size; i += 2 {
			if i+1 > arg_size {
				break
			}

			gCmdArgs.args[os.Args[i]] = os.Args[i+1]
		}
	}

	return gCmdArgs
}

func (this *CmdArgs) String(key, def string) string {
	if _, ok := this.args[key]; !ok {
		return def
	}

	return this.args[key].(string)
}

func (this *CmdArgs) Int64(key string, def int64) int64 {
	if _, ok := this.args[key]; !ok {
		return def
	}

	value, _ := strconv.ParseInt(this.args[key].(string), 10, 64)
	return value
}

func (this *CmdArgs) Int32(key string, def int32) int32 {
	if _, ok := this.args[key]; !ok {
		return def
	}

	value, _ := strconv.ParseInt(this.args[key].(string), 10, 64)
	return int32(value)
}

func (this *CmdArgs) Float64(key string, def float64) float64 {
	if _, ok := this.args[key]; !ok {
		return def
	}

	value, _ := strconv.ParseFloat(this.args[key].(string), 10)
	return value
}

func (this *CmdArgs) Float32(key string, def float32) float32 {
	if _, ok := this.args[key]; !ok {
		return def
	}

	value, _ := strconv.ParseFloat(this.args[key].(string), 10)
	return float32(value)
}
