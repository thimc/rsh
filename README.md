# rsh

Shell interpreter written in go, inspired by [Tom
Duff](https://en.wikipedia.org/wiki/Tom_Duff)'s simple shell (ssh.c).

rsh is capable of:

* simple commands
* file patterns
* quoting ('...' and \c)
* continuation with `\` at end of line
* redirection (`>` and `<`)
* pipelines
* synchronous and asynchronous execution (`;` and `&`)
* conditional execution (`&&` and `||`)
* only built-ins are `cd`, `path` and `exit`
* nothing else

It is very much work in progress so random panics are expected.
It is _not_ ready for daily use just yet.

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

* probably something else
