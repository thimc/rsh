// Command rsh implements a really simple Unix-like shell interpreter.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Cmd interface{}

type Async struct{ Cmd }
type List struct{ Left, Right Cmd }
type Redir struct {
	Cmd
	In, Out string
	Mode    int
}
type Conditional struct {
	Left, Right Cmd
	Success     bool
}
type Pipe struct{ Left, Right Cmd }
type Exec struct{ Args []string }

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
		// log.Printf("Which %q: %q\n", cmd, fp)
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
	return parseline(&ln), nil
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

func parseline(ln *string) Cmd {
	cmd := parsepipe(ln)
	if i := strings.IndexRune(*ln, '&'); i >= 0 {
		*ln = (*ln)[i+1:]
		if i < len(*ln) && (*ln)[i] == '&' {
			*ln = (*ln)[i+1:]
			return Conditional{
				Left:    cmd,
				Right:   parseline(ln),
				Success: true,
			}
		}
		cmd = Async{Cmd: cmd}
	}
	if i := strings.IndexRune(*ln, ';'); i >= 0 {
		*ln = (*ln)[i+1:]
		return List{Left: cmd, Right: parseline(ln)}
	}
	return cmd
}

func parsepipe(ln *string) Cmd {
	cmd := parseexec(ln)
	if i := strings.IndexRune(*ln, '|'); i >= 0 {
		*ln = (*ln)[i+1:]
		if i < len(*ln) && (*ln)[i] == '|' {
			*ln = (*ln)[i+1:]
			return Conditional{
				Left:    cmd,
				Right:   parseline(ln),
				Success: false,
			}
		}
		cmd = Pipe{Left: cmd, Right: parsepipe(ln)}
	}
	return cmd
}

func parseexec(ln *string) Cmd {
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
		return nil
	}
	return Exec{Args: strings.Fields(name)}
}

func mkcmd(args []string, stdin *os.File, stdout *os.File, stderr *os.File) *exec.Cmd {
	if len(args) < 1 {
		panic("empty list")
	}
	var name = args[0]
	if len(args) > 1 {
		args = args[1:]
	} else {
		args = args[:0]
	}
	cmd := exec.Command(which(name), args...)
	// log.Printf("%q with args %q\n", cmd.Path, cmd.Args)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd
}

func run(cmd Cmd, stdin *os.File, stdout *os.File, stderr *os.File) error {
	if cmd == nil {
		return nil
	}
	switch cmd := cmd.(type) {
	case Exec:
		if len(cmd.Args) < 1 {
			return nil
		}
		x := mkcmd(cmd.Args, stdin, stdout, stderr)
		return x.Run()
	case Redir:
		if cmd.In != "" {
			stdin, _ = os.OpenFile(cmd.In, cmd.Mode, 0)
			defer stdin.Close()
		} else if cmd.Out != "" {
			stdout, _ = os.OpenFile(cmd.Out, cmd.Mode, 0)
			defer stdout.Close()
		}
		return run(cmd, stdin, stdout, stderr)
	case List:
		run(cmd.Left, stdin, stdout, stderr)
		run(cmd.Right, stdin, stdout, stderr)
	case Pipe:
		r, w, _ := os.Pipe()
		go func() {
			run(cmd.Left, stdin, w, stderr)
			w.Close()
		}()
		run(cmd.Right, r, stdout, stderr)
		r.Close()
	case Async:
		go run(cmd.Cmd, stdin, stdout, stderr)
	case Conditional:
		err := run(cmd.Left, stdin, stdout, stderr)
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
	var file *os.File
	prompt := true
	switch len(os.Args) {
	case 1:
		file = os.Stdin
	case 2:
		var err error
		file, err = os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: can't open %s\n", os.Args[0], os.Args[1])
			os.Exit(1)
		}
		prompt = false
		defer file.Close()
	default:
		fmt.Fprintf(os.Stderr, "Usage: %s [file]\n", os.Args[0])
		os.Exit(1)
	}
	doprompt(prompt)
	for s = bufio.NewScanner(file); s.Scan(); doprompt(prompt) {
		cmd, err := parse(strings.TrimSuffix(s.Text(), "\n"))
		if err != nil || cmd == nil {
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			continue
		}
		// log.Printf("%#v\n", cmd)
		if err := run(cmd, file, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
