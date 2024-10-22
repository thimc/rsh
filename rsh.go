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

type Async struct{ Cmd Cmd }
type List struct{ Left, Right Cmd }
type Redir struct {
	Cmd     Cmd
	In, Out string
	Mode    int
}
type Conditional struct {
	Left, Right Cmd
	Success     bool
}
type Pipe struct{ Left, Right Cmd }
type Exec struct{ Args []string }

var path = []string{".", "/bin", "/usr/bin", "/usr/local/bin"}

func which(cmd string) string {
	for _, p := range path {
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
	args := strings.Fields(ln)
	switch args[0] {
	case "#":
		return nil, nil
	case "cd":
		if len(args) != 2 {
			return nil, fmt.Errorf("Usage: cd directory")
		}
		return nil, os.Chdir(args[1])
	case "path":
		if len(args) < 2 {
			fmt.Printf("path")
			for _, p := range path {
				fmt.Printf(" %s", p)
			}
			fmt.Println()
		} else {
			path = args[1:]
		}
		return nil, nil
	case "exit":
		if len(args) == 1 {
			os.Exit(0)
		}
		if len(args) == 2 {
			s, _ := strconv.Atoi(args[1])
			os.Exit(s)
		}
		return nil, fmt.Errorf("Usage: exit [status]")
	}
	return parseline(&ln), nil
}

func parseline(ln *string) Cmd {
	cmd := parsepipe(ln)
	if i := strings.IndexRune(*ln, '&'); i >= 0 {
		*ln = (*ln)[i+1:]
		if (*ln)[i] == '&' {
			*ln = (*ln)[i+1:]
			return Conditional{
				Left:    cmd,
				Right:   parseline(ln),
				Success: true,
			}
		}
		cmd = Async{cmd}
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
		if (*ln)[i] == '|' {
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
	return Exec{Args: strings.Fields(name)}
}

func mkcmd(args []string) *exec.Cmd {
	var name = args[0]
	if len(args) > 1 {
		args = args[1:]
	} else {
		args = args[:0]
	}
	cmd := exec.Command(which(name), args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func run(cmd Cmd) error {
	if cmd == nil {
		return nil
	}
	switch cmd := cmd.(type) {
	case Exec:
		c := mkcmd(cmd.Args)
		if c.Path == "" {
			return fmt.Errorf("%s: not found", cmd.Args[0])
		}
		if err := c.Run(); err != nil {
			return err
		}
	case Redir:
		// TODO(thimc): Handle execution of redirection commands
	case List:
		if err := run(cmd.Left); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return run(cmd.Right)
	case Async:
		// TODO(thimc): Handle execution of asynchronous commands
	case Conditional:
		lc, _ := cmd.Left.(Exec)
		lcmd := mkcmd(lc.Args)
		err := lcmd.Run()
		if (err == nil) == cmd.Success {
			rc, _ := cmd.Right.(Exec)
			rcmd := mkcmd(rc.Args)
			return rcmd.Run()
		}
		return nil
	case Pipe:
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		lc, _ := cmd.Left.(Exec)
		lcmd := mkcmd(lc.Args)
		rc, _ := cmd.Right.(Exec)
		rcmd := mkcmd(rc.Args)
		lcmd.Stdout = w
		rcmd.Stdin = r
		for _, err := range []error{lcmd.Start(), rcmd.Start(), w.Close(), r.Close(), lcmd.Wait()} {
			if err != nil {
				return err
			}
		}
		return rcmd.Wait()
	default:
		panic(fmt.Sprintf("unexpected cmd: %#v", cmd))
	}
	return nil
}

func main() {
	var file *os.File
	prompt := true
	doprompt := func() {
		if prompt {
			fmt.Print("% ")
		}
	}
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
	doprompt()
	for s := bufio.NewScanner(file); s.Scan(); doprompt() {
		cmd, err := parse(s.Text())
		if err != nil || cmd == nil {
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
			continue
		}
		//fmt.Printf("%#v\n", cmd)
		if err := run(cmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
