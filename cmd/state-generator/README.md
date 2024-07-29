# State Generator for Unit testing

This code isn't needed for VPC Conf operation, but might be convenient to modify to use in future features when developing large tests.

This CLI was created to automatically create unit tests for the expand AZ feature.

Unit tests for VPC Conf generally had an initial state (which may be VPC Conf state or IP Control state, or both) and an expected state. The expected state is compared with the application state post unit test execution to determine success of the test.

In the case of AZ expansion defining the initial and expected state was many hundreds of lines per test which was not practical to write manually.

This CLI was created to automatically generate initial and expected state and output boiler plate unit test code.

At a high level, this CLI is intended to be run against a local developer environment running the new code that tests are being written for. A new VPC is created, this CLI is executed to grab the current VPC Conf and IPControl state. The tool will then pause. The developer then manually executes the code to be tested. Once VPC Conf has executed the new code, the developer unpauses this CLI which then takes a snapshot of VPC Conf and IPControl state and outputs boilerplate test code using the captured states.

Currently this is only used to build AZ expansion tests, but it could be modified to generate other test code.

