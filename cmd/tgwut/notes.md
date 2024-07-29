## Routes

### List
```
routes_to_add:
  "all":
    - "10.244.96.0/19"
    - "10.128.0.0/16"
    - "10.232.32.0/19"
    - "10.223.126.0/23"
  "prod":
    - "10.252.0.0/22"
    - "10.138.1.0/24"
  "impl":
    - "10.138.132.0/22"
    - "10.131.125.0/24"
    - "10.223.120.0/22"
  "dev/test":
    - "10.138.132.0/22"
    - "10.235.58.0/24"
    - "10.223.120.0/22"
```
### eLDAP notes
10.138.132.0/22 should be added globally in lower envs - it currently is ad-hoc
10.131.125.0/24 (impl), 10.138.1.0/24 (prod) might need to be on all vpcs in all envs

### Old routes
```
You'll see 10.252.0.0/16 in a few places, often times as a blackhole route. That was an old shared service VPC which has since been deleted, so that route doesn't need to be migrated
Not to be confused with 10.252.0.0/22, which is another shared service VPC that just happened to be built in the same IP space
```
