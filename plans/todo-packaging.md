# Packaging and Deployment — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## RPM Package

- [ ] Build and test RPM package (`make package-rpm`)

## DEB Package

- [ ] Verify DEB package builds from the same `nfpm.yaml` (`make package-deb`)

## Docker Compose

- [ ] Verify `docker compose up -d` brings up a working stack from scratch
- [ ] Verify application connects to the Compose-managed PostgreSQL
- [ ] Verify `docker compose down -v` cleanly removes all resources

## ELK Testing Stack

- [ ] Verify `docker compose up -d` in `deploy/elk/` brings up a working ELK stack
- [ ] Verify Logstash picks up NDJSON files and indexes them into Elasticsearch
- [ ] Verify Kibana can query the `chef-migration-metrics` index
- [ ] Verify `docker compose down -v` cleanly removes all ELK resources
- [ ] Keep Logstash pipeline definition up to date when document types change
