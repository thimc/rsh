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

var (
	s    *bufio.Scanner
	path = []string{".", "/bin", "/usr/bin", "/usr/local/bin"}
)

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
	for strings.HasSuffix(ln, "\\") && s.Scan() {
		ln = strings.TrimSuffix(ln, "\\") + s.Text()
	}
	if err := parsebuiltin(&ln); err != nil {
		return nil, err
	}
	if ln == "" {
		return nil, nil
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
			fmt.Printf("path")
			for _, p := range path {
				fmt.Printf(" %s", p)
			}
			fmt.Println()
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
		x := mkcmd(cmd.Args)
		if x.Path == "" {
			return fmt.Errorf("%s: not found", cmd.Args[0])
		}
		if err := x.Run(); err != nil {
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
		acmd, _ := cmd.Cmd.(Exec)
		x := mkcmd(acmd.Args)
		err := x.Start()
		go func() {
			x.Wait()
			fmt.Fprintln(os.Stderr, x.Process.Pid, "exited")
		}()
		fmt.Fprintln(os.Stderr, x.Process.Pid)
		return err
	case Conditional:
		xleft, _ := cmd.Left.(Exec)
		lcmd := mkcmd(xleft.Args)
		if err := lcmd.Run(); cmd.Success == (err == nil) {
			xright, _ := cmd.Right.(Exec)
			rcmd := mkcmd(xright.Args)
			return rcmd.Run()
		}
		return nil
	case Pipe:
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		xleft, _ := cmd.Left.(Exec)
		xright, _ := cmd.Right.(Exec)
		lcmd := mkcmd(xleft.Args)
		rcmd := mkcmd(xright.Args)
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
		//fmt.Printf("%#v\n", cmd)
		if err := run(cmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
