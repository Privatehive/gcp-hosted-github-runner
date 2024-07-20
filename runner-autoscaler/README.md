# Autoscaler

This tiny webserver receives Azure DevOps webhooks for Azure CI jobs. Depending on the job state, the spot instances will be started or stopped.

| Env                | Default      | Description                                                                                  |
| ------------------ | ------------ | -------------------------------------------------------------------------------------------- |
| DEBUG              |              | If env is set, debug logs will be printed (secrets will be leaked)                           |
| PORT               | 8080         | On which port to bind the webserver                                                          |
| AGENTS             |              | A comma separated list of agent names                                                        |
| PROJECT_ID         |              | The Project ID                                                                               |
| ZONE               |              | The zone of the spot instances                                                               |
| ROUTE_POLL         | /poll        | The route that will request job states and start/stop spot instances depending on the state  |
| ROUTE_WEBHOOK      | /webhook     | The route that will receive Azure DevOps job webhooks                                        |
| AUTH_USER          | ci_user      | the HTTP basic auth user                                                                     |
| AUTH_PASSWORD      | <PROJECT_ID> | the HTTP basic auth password - defaults to PROJECT_ID - you should provide a random password |
| AZURE_PAT          |              | A personal access token of a Azure DevOps account                                            |
| AZURE_ORGANIZATION |              | the name of the Azure organization                                                           |
| AZURE_POOL_ID      |              | the ID of the agent pool                                                                     |
