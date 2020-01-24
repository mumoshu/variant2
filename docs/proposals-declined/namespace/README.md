`namespace` was intended for reducing code repetition per group of jobs.

However it turned out that splitting variant command and using top-level `options` and `variables` to share the common parts seemed better in terms of testability.

Proposed but declined usage:

```
namespace "x" {
  option "common1" {

  }
}

job "x a" {

}

job "x b" {

}

namespace "y" {
  option "common2" {

  }
}

job "y c" {

}
```

Alternative:

```
# cmd1/cmd1.hcl

option "common1" {

}

job "a" {

}

job "b" {

}

# cmd2/cmd2.hcl

option "common2" {

}

job "c" {

}
```
