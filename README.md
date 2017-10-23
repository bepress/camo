# camo
Largely a fork of go-camo itself a go implementation of camo

## Developing

### Setup the development environment

After cloning the repo run make develop:

`make develop`


This does three things. First it runs the dependencies target (see dependencies below). Second, it adds a pre-push hook which lints and runs tests before to validate things when pushing to github.

Finally, it also initializes git flow.

### Working on features

To start work on a new feature or bug run:

`git flow feature start <name>`

I usually use the issue number I am working on and a description of the work for the name, e.g. `bp-5064_add-camo-asset-proxy`. This makes it easy for me to recall the issue when necessary and know what it is by description. It also means others know the same thing at a glance.


When done push your branch to your github fork and create a PR. Once that is approved you can run:

`git flow feature finish <name>`

This will merge the feature branch into develop and delete the local feature branch. Then you can push your changes to develop


### Checking work as you go.

#### Lint

You can run various static analysis tools with the lint target.

`make lint`

This will report various issues in your code.

#### Testing

`make test` will run the tests for you.

`make test-race` will run the tests with the race detector on. This slows the test run a bit as it instruments the compiled code to do the detection which slows the test run noticably. It is normal to run `make test` regularly and then run with the race detector before committing and pushing to ensure you haven't added any detectable races to the code.

To check on test coverage run:

`make coverage`

Or to see line by line coverage in html run

`make coverage-html`


### Building

`make build` will build a binary you can run on your dev workstation for testing purposes.

`make build-linux` will cross compile a linux binary for testing on linux should you need to.

`make master` and `make release` targets are for circle to do deployments.



## Dependencies

To install dependencies for building artifacts run:

`make dependencies`

This is done by the develop target as well. So you probably don't need to do this you're self. It is a separate target so that CI/CD can setup dependencies in a non-development environment.
