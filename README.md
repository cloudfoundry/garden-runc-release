# Guardian-Release

A [BOSH](http://docs.cloudfoundry.org/bosh/) release for deploying
[Guardian](https://github.com/cloudfoundry-incubator/guardian).

# Developing

1. Check out/update the submodules

    `git submodule update --init --recursive`

1. Set your GOPATH to the checked out directory, or use direnv to do this, as
   below

    `direnv allow`

1. Write code in a submodule

    ~~~~
    cd src/github.com/cloudfoundry-incubator/guardian # for example
    # test, code, test..
    git commit
    git push
    ~~~~

1. Run all the tests

    `./scripts/test`

1. Create a bump commit

    `./scripts/bump`
