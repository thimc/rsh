# rsh

Really simple shell written in go, inspired by [Tom Duff](https://en.wikipedia.org/wiki/Tom_Duff)'s "ssh.c".

RSH is WIP so missing functions and random panics are normal, and are to
be expected.  It is _not_ ready daily use just yet.

Grammar is roughly:

	cmd:	simple
	|	simple | cmd
	|	simple ; cmd
	|	simple & cmd
	|	simple && cmd
	|	simple || cmd
	simple:
	|	simple word
	|	simple < word
	|	simple > word
	|	simple &


## TODO

* Finish up the parser, the redirection bit is still missing.

* Squash the last bugs that could cause panics

Besides whats there now I want to add:

* file pattern matching, i.e globbing
	ls /h*/
would expand to
	ls /home/

* Quoting '...'

* Continuation with a \ at the end of the line
