# Release Admission

## Gate

`scripts/check-release-admission.sh <version>`은 다음을 순서대로 실행한다.

1. semantic version과 major channel 판정.
2. full Go/repository gate.
3. hardened container build/restart gate.
4. repeated performance/recovery SLO gate.
5. release binary/checksum build gate.
6. repository publication boundary gate.

push, tag, release와 visibility 변경은 이 local admission과 별개의 외부-write 승인이다. public push 전 machine-local private-inventory gate도 별도로 필요하다.

## Alpha Admission

manifest의 `alpha_admitted: true`는 `v0.*` candidate가 위 gate를 통과하면 local release admission을 얻는다는 뜻이다. alpha API와 service는 compatibility를 약속하지 않으며 release note에 experimental boundary와 known limits를 유지한다.

## Stable Admission

major version 1 이상은 manifest의 exact `stable_admitted: true`가 추가로 필요하다. 현재 값은 false이며 다음 blocker가 남아 있다.

- stable Go API와 wire compatibility/deprecation policy.
- authenticated and encrypted remote transport 또는 repository-owned approved gateway contract.
- 실제 external consumer integration과 target workload evidence.
- production retention/backup authority와 support ownership.

따라서 P10은 stable release를 수행한 것이 아니라 stable release가 실수로 수행되지 않도록 executable gate를 완성한 것이다. blocker를 해결할 때는 관련 compatibility/security/operations decision, tests와 external evidence를 먼저 추가하고 마지막 change에서 admission flag를 바꾼다.

## Commands

```sh
scripts/check-release-admission.sh v0.1.0
scripts/check-release-admission.sh v1.0.0
```

첫 command는 모든 local gate를 실행한다. 두 번째 command는 현재 stable blocker를 안내하며 artifact build나 publication 전에 실패한다.
