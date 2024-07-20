# g-spot-runner-github-actions

#### Proof of concept - not fit for production, not maintained

## Troubleshooting

```
Error applying IAM policy for cloudrun service "v1/projects/azure-pipelines-spot-agent/locations/us-east1/services/cloudrun-service": Error setting IAM policy for cloudrun service "v1/projects/azure-pipelines-spot-agent/locations/us-east1/services/cloudrun-service": googleapi: Error 400: One or more users named in the policy do not belong to a permitted customer,  perhaps due to an organization policy
```

Solution by Project tags: [How to create public Cloud Run services when Domain Restricted Sharing is enforced](https://cloud.google.com/blog/topics/developers-practitioners/how-create-public-cloud-run-services-when-domain-restricted-sharing-enforced?hl=en)

or override the Organization Policy "Domain Restricted Sharing" in the project, by setting it to "Allow all".

## Warning

I am not responsible if this Terraform module results in high costs on your billing account. Keep an eye on your billing account.
