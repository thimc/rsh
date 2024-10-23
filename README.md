# rsh

Really simple shell interpreter written in go, inspired by [Tom
Duff](https://en.wikipedia.org/wiki/Tom_Duff)'s "ssh.c".

rsh is capable of the following:

* simple commands
* file patterns
* quoting ('...' and \c)
* continuation with `\` at end of line
* redirection
* pipelines
* synchronous and asynchronous execution (`;` and `&`)
* conditional execution (`&&` and `||`)
* only built-ins are `cd`, `path` and `exit`
* nothing else

It is very much work in progress so missing functions and random panics
are normal, and are to be expected.  It is _not_ ready daily use just
yet.

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
	|	simple < word1 > word2
	|	simple &

## TODO

* finish up the quoting functionality, escaping characters is not
  working as expected.
* file pattern matching, i.e globbing `ls /h*/` would expand to `ls /home/`
