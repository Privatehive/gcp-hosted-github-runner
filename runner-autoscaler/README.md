# Autoscaler

This tiny webserver receives GitHub "Workflow jobs" webhook events. Depending on the workflow job state, a compute instance will be started or deleted.
The short timeout of the GitHub webhook (10 sec) has to be worked around (10 sec are not enough to start compute instance) by using a Clout Task queue that calls the webserver back with an increased timeout.

| Env                   | Default         | Description                                                                                 |
| --------------------- | --------------- | ------------------------------------------------------------------------------------------- |
| ROUTE_WEBHOOK         | /webhook        | The Cloud Run path that is invoked by the GitHub webhook                                    |
| ROUTE_DELETE_RUNNER   | /delete_runner  | The Cloud Run callback path invoked by Cloud Task when a compute instance should be deleted |
| ROUTE_CREATE_RUNNER   | /create_runner  | The Cloud Run callback path invoked by Cloud Task when a compute instance should be created |
| WEBHOOK_SECRET        | arbitrarySecret | The GitHub webhook secret                                                                   |
| PROJECT_ID            |                 | The Google Cloud Project id                                                                 |
| ZONE                  |                 | The Google Cloud zone where the spot compute instance should be created                     |
| TASK_QUEUE            |                 | The URL of the Cloud Task queue                                                             |
| INSTANCE_TEMPLATE_URL |                 | The URL of the compute instance template                                                    |
| RUNNER_PREFIX         | runner          | Prefix of the compute instances (a random string will be added to the name)                 |
| RUNNER_GROUP          | Default         | The GitHub runner group                                                                     |
| PORT                  | 8080            | On which port to bind the webserver                                                         |
