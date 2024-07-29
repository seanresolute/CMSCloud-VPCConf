# CMSNet API integration

One of the things we support is adding connections to CMSNet, which involves attaching a transit gateway, adding routes to the transit gateway, and adding routes to subnet route tables. We don't do these changes the same way as other infrastructure changes. Rather, we use the [babygroot API](https://github.cms.gov/CCSVDC-Infrastructure/groot/tree/master/babygroot_api), which has its own independent processes for doing tasks, monitoring their status, and checking the status of deployments. The server endpoints related to this mostly just make babygroot API calls and then report the results and/or integrate the results with other information from the database.

The babygroot API also edits route tables but, because we ignore routes that don't match any destinations we care about, it should not cause any conflicts.
