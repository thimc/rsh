// Command rsh implements a really simple Unix-like shell interpreter.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Cmd interface{}

type file struct {
	Path string
	Mode int
}

type Async struct{ Cmd }
type List struct{ Left, Right Cmd }
type Redir struct {
	Cmd
	In, Out file
}
type Conditional struct {
	Left, Right Cmd
	Success     bool
}
type Pipe struct{ Left, Right Cmd }
type Exec struct{ Args []string }

var verbose = flag.Bool("v", false, "enables verbose mode")
var (
	s    *bufio.Scanner
	path = []string{".", "/bin", "/usr/bin", "/usr/local/bin"}
)

func which(cmd string) string {
	var err error
	for _, p := range path {
		if p == "." {
			p, err = os.Getwd()
			if err != nil {
				p = ""
			}
		}
		fp := filepath.Join(p, cmd)
		if _, err := os.Stat(fp); err == nil {
			return fp
		}
	}
	return ""
}

func parse(ln string) (Cmd, error) {
	if ln == "" {
		return nil, nil
	}
	for strings.HasSuffix(ln, "\\") && s.Scan() {
		ln = strings.TrimSuffix(ln, "\\") + s.Text()
	}
	if err := parsebuiltin(&ln); err != nil {
		return nil, err
	}
	return parseline(&ln)
}

func parsebuiltin(ln *string) error {
	args := strings.Fields(*ln)
	switch args[0] {
	case "#":
		*ln = ""
		return nil
	case "cd":
		if len(args) != 2 {
			return fmt.Errorf("Usage: cd directory")
		}
		*ln = ""
		return os.Chdir(args[1])
	case "path":
		if len(args) < 2 {
			fmt.Println("path", strings.Join(path, " "))
		} else {
			path = args[1:]
		}
		*ln = ""
		return nil
	case "exit":
		if len(args) == 1 {
			os.Exit(0)
		}
		if len(args) == 2 {
			s, _ := strconv.Atoi(args[1])
			os.Exit(s)
		}
		return fmt.Errorf("Usage: exit [status]")
	}
	return nil
}

func parseline(ln *string) (Cmd, error) {
	cmd, err := parsepipe(ln)
	if i := strings.IndexRune(*ln, '&'); i >= 0 {
		*ln = (*ln)[i+1:]
		if i < len(*ln) && (*ln)[i] == '&' {
			*ln = (*ln)[i+1:]
			right, err := parseline(ln)
			if err != nil {
				return nil, err
			}
			return Conditional{Left: cmd, Right: right, Success: true}, nil
		}
		cmd = Async{Cmd: cmd}
	}
	if i := strings.IndexRune(*ln, ';'); i >= 0 {
		*ln = (*ln)[i+1:]
		right, err := parseline(ln)
		if err != nil {
			return nil, err
		}
		return List{Left: cmd, Right: right}, nil
	}
	return cmd, err
}

func parsepipe(ln *string) (Cmd, error) {
	cmd, err := parseexec(ln)
	if i := strings.IndexRune(*ln, '|'); i >= 0 {
		*ln = (*ln)[i+1:]
		if i < len(*ln) && (*ln)[i] == '|' {
			*ln = (*ln)[i+1:]
			right, err := parseline(ln)
			if err != nil {
				return nil, err
			}
			return Conditional{Left: cmd, Right: right, Success: false}, nil
		}
		right, err := parseline(ln)
		if err != nil {
			return nil, err
		}
		cmd = Pipe{Left: cmd, Right: right}
	}
	return cmd, err
}

func parseexec(ln *string) (Cmd, error) {
	var name string
	for i, r := range *ln {
		switch r {
		case '|', '&', ';':
			*ln = (*ln)[i:]
			goto done
		default:
			name += string(r)
		}
	}
done:
	if name == "" {
		return nil, nil
	}
	cmd := Exec{Args: fields(name)}
	if strings.IndexAny(name, "<>") >= 0 {
		return parseredirs(name)
	}
	return cmd, nil
}

func parseredirs(args string) (Redir, error) {
	var rcmd Redir
	for {
		i := strings.IndexAny(args, "<>")
		if i < 0 {
			break
		}
		r := args[i]
		start := i + 1
		for start < len(args) && (args[start] == ' ' || args[start] == r) {
			start++
		}
		end := strings.IndexAny(args[start:], " ")
		if end < 0 {
			end = len(args)
		} else {
			end += start
		}
		path := strings.TrimSpace(args[start:end])
		args = args[:i] + args[end:]
		rcmd.Cmd = Exec{Args: fields(args)}
		switch r {
		case '>':
			rcmd.Out = file{Path: path, Mode: os.O_WRONLY | os.O_CREATE | os.O_TRUNC}
		case '<':
			rcmd.In = file{Path: path, Mode: os.O_RDONLY}
		}
	}
	return rcmd, nil
}

func fields(s string) []string {
	var (
		list    []string
		current string
		quoted  = false
	)
	for i := 0; i < len(s); i++ {
		r := s[i]
		switch r {
		case '\'':
			quoted = !quoted
		case ' ':
			if quoted {
				current += string(r)
			} else {
				if len(current) > 0 {
					list = append(list, current)
					current = ""
				}
			}
		default:
			current += string(r)
		}
	}
	if len(current) > 0 {
		list = append(list, current)
	}
	return list
}

func mkcmd(args []string, stdin *os.File, stdout *os.File, stderr *os.File) *exec.Cmd {
	var name = args[0]
	if len(args) > 1 {
		args = args[1:]
	} else {
		args = args[:0]
	}
	cmd := exec.Command(which(name), args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd
}

func run(cmd Cmd, stdin *os.File, stdout *os.File, stderr *os.File) error {
	if cmd == nil {
		return nil
	}
	var err error
	switch cmd := cmd.(type) {
	case Exec:
		if len(cmd.Args) < 1 {
			return nil
		}
		return mkcmd(cmd.Args, stdin, stdout, stderr).Run()
	case Redir:
		if cmd.In.Path != "" {
			stdin, err = os.OpenFile(cmd.In.Path, cmd.In.Mode, 0)
			if err != nil {
				return nil
			}
			defer stdin.Close()
		}
		if cmd.Out.Path != "" {
			stdout, err = os.OpenFile(cmd.Out.Path, cmd.Out.Mode, 0)
			if err != nil {
				return nil
			}
			defer stdout.Close()
		}
		return run(cmd.Cmd, stdin, stdout, stderr)
	case List:
		run(cmd.Left, stdin, stdout, stderr)
		run(cmd.Right, stdin, stdout, stderr)
	case Pipe:
		r, w, err := os.Pipe()
		if err != nil {
			return nil
		}
		go func() {
			run(cmd.Left, stdin, w, stderr)
			w.Close()
		}()
		run(cmd.Right, r, stdout, stderr)
		r.Close()
	case Async:
		go run(cmd.Cmd, stdin, stdout, stderr)
	case Conditional:
		err = run(cmd.Left, stdin, stdout, stderr)
		if cmd.Success == (err == nil) {
			return run(cmd.Right, stdin, stdout, stderr)
		}
	default:
		panic(fmt.Sprintf("unexpected main.Cmd: %#v", cmd))
	}
	return nil
}

func doprompt(p bool) {
	if p {
		fmt.Print("% ")
	}
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [file]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	var file *os.File
	prompt := true
	switch args := flag.Args(); flag.NArg() {
	case 0:
		file = os.Stdin
	case 1:
		var err error
		file, err = os.Open(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: can't open %s\n", os.Args[0], args[0])
			os.Exit(1)
		}
		prompt = false
		defer file.Close()
	default:
		flag.Usage()
		os.Exit(1)
	}
	doprompt(prompt)
	for s = bufio.NewScanner(file); s.Scan(); doprompt(prompt) {
		cmd, err := parse(strings.TrimSuffix(s.Text(), "\n"))
		if cmd != nil && *verbose {
			fmt.Printf("%#v\n", cmd)
		}
		if err != nil {
			goto printerr
		} else if err := run(cmd, file, os.Stdout, os.Stderr); err != nil {
			goto printerr
		}
		continue
	printerr:
		fmt.Fprintln(os.Stderr, err)
	}
}
