package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/Esgorhannoth/gelo"
	"github.com/Esgorhannoth/gelo/commands"
	"github.com/Esgorhannoth/gelo/extensions"
	"os"
	"regexp"
	"strings"
	"unicode"
)

//globals and helper functions

var history []string
var to_exit, metainvoke bool
var stdin = bufio.NewReader(os.Stdin)
var no_prelude = flag.Bool("no-prelude", false, "do not load prelude.gel")

func check(failmsg string, e error) {
	if e != nil {
		fmt.Println(failmsg)
		fmt.Println(e.Error())
		os.Exit(1)
	}
}

//interpreter metacommands

func exit(_ *gelo.VM, _ *gelo.List, ac uint) gelo.Word {
	if ac != 0 {
		return metahelp("exit")
	}
	to_exit = true
	return gelo.Null
}

func run(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	if ac == 0 {
		return metahelp("run")
	}
	fname := args.Value.Ser().String()
	file, err := os.Open(fname)
	defer file.Close()
	if err != nil {
		return gelo.StrToSym(
			"Could not open file " + fname + "\n" + err.Error())
	}
	ret, err := vm.Run(file, args.Next)
	if err != nil {
		//XXX unclear why this type assertion is necessary as
		//gelo.Error satisfies os.Error and gelo.Word yet without the assertion
		//the comppiler complains that:
		//"cannot use err (type os.Error) as type gelo.Word in return argument:
		//os.Error does not implement gelo.Word (missing Copy method)"
		return err.(gelo.Word)
	}
	return ret
}

func load(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	if ac != 1 {
		return metahelp("load")
	}
	fname := args.Value.Ser().String()
	file, err := os.Open(fname)
	defer file.Close()
	if err != nil {
		return gelo.StrToSym(
			"Could not open file " + fname + "\n" + err.Error())
	}

	buffer := make([]byte, 4096)
	llines := NewReadline()
	n := 0
	for err == nil {
		n, err = file.Read(buffer)
		buffer = buffer[:n]
		if n > 0 {
			llines.Read(buffer)
		}
	}
	for _, line := range llines.lines {
		history = append(history, line)
	}
	return gelo.Null
}

func save(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	if ac != 1 {
		return metahelp("save")
	}
	fname := args.Value.Ser().String()
	file, err := os.OpenFile(fname, os.O_WRONLY|os.O_CREATE, 0664)
	defer file.Close()
	if err != nil {
		return gelo.StrToSym(
			"Could not open file " + fname + "\n" + err.Error())
	}
	for _, line := range history {
		if _, err := file.WriteString(line); err != nil {
			fmt.Println("Error writing file\n" + err.Error())
			return gelo.Null
		}
	}
	return gelo.Null
}

func clear(_ *gelo.VM, _ *gelo.List, ac uint) gelo.Word {
	if ac != 0 {
		return metahelp("clear")
	}
	history = history[:0]
	return gelo.Null
}

func rewind(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	if ac > 1 {
		return metahelp("rewind")
	} else if ac == 0 {
		if len(history) > 0 {
			history = history[:len(history)-1]
		}
		return gelo.Null
	}
	n := _valid_idx(vm, "rewind", args.Value)
	if n == 0 {
		_invalid_idx(vm, "rewind", gelo.Null)
	}
	history = history[:len(history)-n]
	return gelo.Null
}

func _foreshorten(line string) string {
	out := line
	for i, c := range line {
		if (c == '\n' && i+1 != len(line)) || i > 70 {
			out = strings.TrimRightFunc(line[:i], unicode.IsSpace) + "..."
			break
		} else if c == '\n' {
			//last is newline
			out = line[:i]
		}
	}
	return out
}

func show_history(_ *gelo.VM, _ *gelo.List, ac uint) gelo.Word {
	if ac != 0 {
		return metahelp("history")
	}
	for i, line := range history {
		fmt.Println(i, "->", _foreshorten(line))
	}
	return gelo.Null
}

func search(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	if ac != 1 {
		return metahelp("search")
	}
	re, err := regexp.Compile(args.Value.Ser().String())
	if err != nil {
		return gelo.StrToSym(
			"Regex couldn't compile: " + err.Error() + "\n" + dollar_map["search"].help)
	}
	for i, line := range history {
		if re.MatchString(line) {
			fmt.Println(i, "->", _foreshorten(line))
		}
	}
	return gelo.Null
}

func _invalid_idx(vm *gelo.VM, name string, w gelo.Word) {
	gelo.RuntimeError(vm,
		"invalid index "+w.Ser().String()+"\n"+dollar_map[name].help)
}

func _valid_idx(vm *gelo.VM, name string, w gelo.Word) int {
	var n *gelo.Number
	var ok bool
	n, ok = w.(*gelo.Number)
	if !ok {
		_invalid_idx(vm, name, w)
	}
	i64, ok := n.Int()
	if !ok {
		_invalid_idx(vm, name, n)
	}
	i := int(i64)
	if i < 0 || i >= len(history) {
		_invalid_idx(vm, name, n)
	}
	return i
}

var _slice = extensions.MakeArgParser("i ['to j]?")

func _make_slice(vm *gelo.VM, name string, args *gelo.List) (i, j int) {
	//XXX ugly hack because 'all|[i ['to j]?] isn't working and I am terrible
	if args.Len() == 1 && args.Value.Ser().String() == "all" {
		return 0, len(history)
	}
	Args, ok := _slice(args)
	if !ok {
		gelo.RuntimeError(vm, "invalid arguments\n"+dollar_map[name].help)
	}
	/*if _, all := Args["all"]; all {
	    return 0, len(history)
	}*/
	i = _valid_idx(vm, name, Args["i"])
	j = i
	if J, ok := Args["j"]; ok {
		j = _valid_idx(vm, name, J)
	}
	j++
	if i >= j {
		gelo.RuntimeError(vm,
			"invalid interval, i >= j\n"+dollar_map[name].help)
	}
	return
}

func replay(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	// TODO crashes
	if ac > 1 {
		return metahelp("replay")
	}
	var lines []string
	if ac == 0 {
		ln := len(history)
		lines = history[ln-1 : ln]
	} else {
		i, j := _make_slice(vm, "replay", args)
		lines = history[i:j]
	}
	metainvoke = false
	defer func() { metainvoke = true }()
	for _, line := range lines {
		play(vm, line)
	}
	return gelo.Null
}

func cut(vm *gelo.VM, args *gelo.List, _ uint) gelo.Word {
	i, j := _make_slice(vm, "cut", args)
	history = append(history[:i], history[j:]...)
	return gelo.Null
}

func see(vm *gelo.VM, args *gelo.List, _ uint) gelo.Word {
	i, j := _make_slice(vm, "see", args)
	for _, line := range history[i:j] {
		fmt.Print(line)
	}
	return gelo.Null
}

var _trace_parser = extensions.MakeArgParser(
	"'on|'off 'runtime? 'parser? 'alien? 'system?")

func trace(vm *gelo.VM, args *gelo.List, _ uint) gelo.Word {
	Args, ok := _trace_parser(args)
	if !ok {
		return metahelp("trace")
	}
	set := func(name string) bool {
		_, ok := Args[name]
		return ok
	}
	//if none are set turn them all on (or off)
	all := !set("alien") && !set("runtime") && !set("system") && !set("parser")
	//default is on, so we only care if off is set
	if set("off") {
		if all {
			gelo.TraceOff(gelo.All_traces)
		} else {
			if set("runtime") {
				gelo.TraceOff(gelo.Runtime_trace)
			}
			if set("parser") {
				gelo.TraceOff(gelo.Parser_trace)
			}
			if set("alien") {
				gelo.TraceOff(gelo.Alien_trace)
			}
			if set("system") {
				gelo.TraceOff(gelo.System_trace)
			}
		}
	} else { //either on specified or none specified
		if all {
			gelo.TraceOn(gelo.All_traces)
		} else {
			if set("runtime") {
				gelo.TraceOn(gelo.Runtime_trace)
			}
			if set("parser") {
				gelo.TraceOn(gelo.Parser_trace)
			}
			if set("alien") {
				gelo.TraceOn(gelo.Alien_trace)
			}
			if set("system") {
				gelo.TraceOn(gelo.System_trace)
			}
		}
	}
	return gelo.Null
}

func metahelp(name string) gelo.Word {
	fmt.Println(dollar_map[name].help)
	return gelo.Null
}

func help(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	name := "help"
	if ac == 1 {
		name = args.Value.Ser().String()
	}
	c, ok := dollar_map[name]
	if !ok {
		fmt.Println("Unknown command: " + name)
		c = dollar_map["help"]
	}
	fmt.Println(c.help)
	return gelo.Null
}

// build urmetacommand

type command struct {
	help string
	call func(*gelo.VM, *gelo.List, uint) gelo.Word
}

var dollar_map = map[string]command{}

func init() {
	dollar_map["exit"] = command{
		"exit\n\tExit the interpreter",
		exit,
	}
	dollar_map["quit"] = command{
		"exit\n\tExit the interpreter",
		exit,
	}
	dollar_map["help"] = command{
		"help command?\n\tDisplays help for an interpreter command. Enter \"$$ list\" to list commands",
		help,
	}
	dollar_map["run"] = command{
		"run file-name arguments*\n\tExecute program in file-name with specified arguments",
		run,
	}
	dollar_map["history"] = command{
		"history\n\tDisplay a brief, numbered history of all commands entered",
		show_history,
	}
	dollar_map["hist"] = command{
		"history\n\tDisplay a brief, numbered history of all commands entered",
		show_history,
	}
	dollar_map["search"] = command{
		"search regex\n\tSearch history against regex and display brief, numbered history of all matches.",
		search,
	}
	dollar_map["clear"] = command{
		"clear\n\tClear history (leaves namespace intact)",
		clear,
	}
	dollar_map["rewind"] = command{
		"rewind\n\tRemove last entry from history",
		rewind,
	}
	dollar_map["save"] = command{
		"save file-name\n\tSave current history into file-name.",
		save,
	}
	dollar_map["load"] = command{
		"load file-name\n\tStore in history each line in file-name",
		load,
	}
	dollar_map["replay"] = command{
		"replay n?\n\tExecute nth command in history or last command if n is unspecified",
		replay,
	}
	dollar_map["cut"] = command{
		"cut i ['to j]?\n\tRemove lines i to j from history or just line i if j is unspecified",
		cut,
	}
	dollar_map["see"] = command{
		"see i ['to j]?\n\tDisplay in full lines i to j from history or just line i if j is unspecified",
		see,
	}
	dollar_map["trace"] = command{
		"trace ['on|'off] 'runtime? 'parser? 'alien? 'system?\n\tTurn on or off a set of traces. If no traces are specified, the default is all of them. Due to limitation in the Argument parser traces must be specified in the same order as listed.",
		trace,
	}

	var acc []string
	for k, _ := range dollar_map {
		acc = append(acc, "\t"+k)
	}
	acc = append(acc, "\tlist")

	dollar_map["list"] = command{
		"list\n\tList all interpreter commands",
		func(_ *gelo.VM, _ *gelo.List, ac uint) gelo.Word {
			if ac != 0 {
				metahelp("list")
			}
			fmt.Println("$$ responds to the following commands:")
			for _, v := range acc {
				fmt.Println(v)
			}
			return gelo.Null
		},
	}
}

func Dollar(vm *gelo.VM, args *gelo.List, ac uint) gelo.Word {
	var cmd command
	var there bool
	if ac == 0 {
		fmt.Println("No command specified")
		cmd = dollar_map["help"]
		args = nil
	} else {
		name := args.Value.Ser().String()
		if cmd, there = dollar_map[name]; !there {
			fmt.Println("Unknown command:", name)
			cmd = dollar_map["help"]
		}
		args = args.Next
		ac--
	}
	metainvoke = true
	return cmd.call(vm, args, ac)
}

type Readline struct {
	buffer                          *bytes.Buffer
	lines                           []string
	literal, escaped, star, comment bool
	clause, quote, first            int
}

func NewReadline() *Readline {
	r := &Readline{}
	r.Reset()
	return r
}

func (r *Readline) Reset() {
	r.buffer = new(bytes.Buffer)
	r.first = -1
	r.lines = r.lines[:0]
}

func (r *Readline) IsComplete() bool {
	return !r.literal && !r.star && r.clause == 0 && r.quote == 0
}

//func (r *Readline) Iter() <-chan string {
//	return r.lines.Iter()
//}

func (r *Readline) Read(p []byte) (n int, _ error) {
	for i, c := range p {
		if r.first == -1 {
			switch c {
			case ' ', '\t', '\f', '\r':

			default:
				r.first = i
			}
		}
		if r.comment {
			if !r.escaped {
				switch c {
				case '{':
					r.quote++
				case '}':
					r.quote--
				case '\\':
					r.escaped = true
				case '\n':
					r.comment = r.quote == 0
				}
			} else {
				r.escaped = false
			}
		} else if r.star {
			switch c {
			case ' ', '\t', '\n', '\f', '\r':
				r.buffer.WriteByte(c)
				continue
			default:
				r.star = false
			}
		} else if !r.escaped {
			if !r.literal {
				switch c {
				case '{':
					r.quote++
				case '}':
					r.quote--
				case '[':
					r.clause++
				case ']':
					r.clause--
				case '"':
					r.literal = true
				case '#':
					r.comment = r.first == i
				case '\\':
					r.escaped = true
				}
			} else if c == '"' {
				r.literal = false
			}
		} else {
			r.escaped = false
			r.star = c == '*' && r.quote == 0
		}

		r.buffer.WriteByte(c)
		if !r.escaped && (c == '\n' || c == ';') && r.IsComplete() {
			if !(c == '\n' && r.first == i) { //ignore blank lines
				r.lines = append(r.lines, r.buffer.String())
			}
			r.buffer.Reset()
			r.first = -1
		}
	}

	n, r.first = len(p), -1
	return
}

func play(vm *gelo.VM, line string) {
	if ret, err := vm.Run(strings.NewReader(line), nil); err == nil {
		//don't bother showing ""
		if r := ret.Ser().String(); len(r) != 0 {
			fmt.Println("=> ", ret.Ser().String())
		}

		if !metainvoke {
			//execution was succesful so save in history
			history = append(history, line)
		}
	} else {
		fmt.Println("Failed with:", err.Error())
	}
	metainvoke = false
}

func main() {
	flag.Parse()

	vm := gelo.NewVM(extensions.Stdio)
	defer vm.Destroy()

	gelo.SetTracer(extensions.Stderr)

	vm.Register("$", Dollar)
	vm.RegisterBundle(gelo.Core)
	vm.RegisterBundles(commands.All)

	if !*no_prelude {
		prelude, err := os.Open("prelude.gel")
		defer prelude.Close()
		check("Could not open prelude.gel", err)

		_, err = vm.Run(prelude, nil)
		check("Could not load prelude", err)
	}

	llines := NewReadline()
	for {
		first := true
		//grab one (or more if ; is used) logical lines from stdin
		for {
			if to_exit {
				break
			}
			if first {
				fmt.Print(">> ")
				first = false
			} else {
				fmt.Print(".. ")
			}
			pline, err := stdin.ReadSlice('\n')
			to_exit = err != nil
			llines.Read(pline)
			if llines.IsComplete() {
				break
			}
		}
		for _, lline := range llines.lines {
			play(vm, lline)
		}
		//there's a semilegitimate reason we check this twice instead of
		//breaking out all at once: if we don't and there's an unbalanced
		//bracket before EOF, it will loop printing ".. ". A better fix would be
		//to redesign Readline to handle the err from stdin but then it couldn't
		//be a reader. This will require some thought.
		if to_exit {
			break
		}
		llines.Reset()
	}
	fmt.Println()
}
