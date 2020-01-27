`boilerplate` was intended for reducing code repetition across jobs.


However it turned out that splitting variant command and using top-level `options` and `variables` to share the common parts seemed better interms of testability.

Proposed but declined usage:

```
boilerplate "x" {
  option "common1" {

  }
}

job "a" {
  boilerplate = "x"
}

job "b" {
  boilerplate = "x"
}

boilerplate "y" {
  option "common2" {

  }
}

job "c" {
  boilerplate = "y"
}
```

Alternative:

```
# cmd1/cmd1.variant

option "common1" {

}

job "a" {

}

job "b" {

}

# cmd2/cmd2.variant

option "common2" {

}

job "c" {

}
```
