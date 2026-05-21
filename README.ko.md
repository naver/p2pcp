# p2pcp

고성능 P2P 파일 복제/배포 도구입니다. 하나의 데이터 소스를 다수 노드로 동시에, 효율적이고 안정적으로 배포하는 데 유용합니다.

## 특징
- **대규모 병렬 전송**: 피어 간 분산 전송으로 단일 스토리지 병목을 최소화하고, 노드 수 확장에 따라 처리량 증가를 기대할 수 있습니다.
- **다양한 디스커버리**: HTTP URL(CSV), DNS SRV/A, 외부 레지스트리(하트비트 기반)를 지원합니다.
- **신뢰성과 관측성**: 무결성 검증(해시), 유휴/대기 타임아웃, 통계 출력 지원.
- **유연한 경로**: 로컬 파일시스템을 지원하며, zstd 압축으로 대역폭을 최적화할 수 있습니다.
- **운영 친화**: 간단한 CLI, 로그/통계로 문제 진단 용이, Kubernetes 환경과 궁합이 좋습니다.

## 빠른 시작
- 시드(파일 서버) 시작
```bash
p2pcp -src /data/files
```

- P2P 다운로드
```bash
p2pcp -dst /data/dst -peer-list seeder:10090,peer1:10090,peer2:10090
```

- 로컬 복사 + P2P(공유 스토리지 병목 완화)
```bash
p2pcp -src /mnt/nfs/src -dst /data/cache -peer-list peer1:10090,peer2:10090
```

## 사용 예시
- 로컬 전송(압축/청크/동시성/검증/통계)
```bash
p2pcp -src /src -dst /dst \
  -compress-type zstd -chunk-size 8388608 \
  -local-num-concurrent 8 -verify-on-complete -statistics
```

- 다수 피어 확장(시드/피어)
```bash
# 시드(파일 서버)
p2pcp -src /src

# 피어 1
p2pcp -dst /dst -peer-list seeder,peer1,peer2,peer3 -exit-complete
# 피어 2
p2pcp -dst /dst -peer-list seeder,peer1,peer2,peer3 -exit-complete
# 피어 3
p2pcp -dst /dst -peer-list seeder,peer1,peer2,peer3 -exit-complete
```

- Kubernetes 환경(Headless Service 기반 디스커버리)
```bash
# DNS SRV 레코드 (포트 정보 포함)
p2pcp -dst /dst -peer-list-srv p2pcp-service.default.svc.cluster.local -exit-complete

# DNS A 레코드 (고정 포트 10090 사용)
p2pcp -dst /dst -peer-list-a p2pcp-service.default.svc.cluster.local -exit-complete
```

## 디스커버리 전략
- **정적 목록** (`-peer-list`): 쉼표로 구분된 여러 주소를 직접 입력. 동적 제공자와 항상 병합됩니다.
- **동적 제공자** (상호 배타적): `-peer-list-url`, `-peer-list-srv`, `-peer-list-a` 중 하나만 지정.
  - **HTTP URL** (`-peer-list-url`): HTTP 엔드포인트 1개 → 여러 피어를 CSV 형식으로 반환.
  - **DNS SRV** (`-peer-list-srv`): FQDN 1개 → 여러 피어 반환 (포트 정보 포함)
    - 일반 DNS: `_<service>._<proto>.<name>` 형식 필수
    - Kubernetes: Service FQDN만으로도 가능 (자동 처리)
  - **DNS A** (`-peer-list-a`): FQDN 1개 → 여러 IP 반환 (고정 포트 10090 사용)
- **폴링 간격**: `-peer-list-poll-interval`(기본 3s)로 동적 제공자(URL/SRV/A) 재조회 주기 조절.

## 성능 튜닝 팁
- 동시성
  - `-peer-num-concurrent`: 피어별 동시 전송 작업 수(기본 2, 1~16)
  - `-local-num-concurrent`: 로컬 동시 작업 수(기본 1, 1~16)
- 압축: `-compress-type`은 `none|zstd`. WAN/저대역에서는 zstd가 유리할 수 있습니다.
- 청크 크기: `-chunk-size` 기본 16MiB. 8MiB~128MiB 범위에서 워크로드, 네트워크, 메모리 상황에 맞춰 조정하세요. (최대 128MiB)
- 파일 그룹화: `-files-per-chunk`로 청크당 최대 파일 수를 제어합니다(기본 5)
- 유휴/대기/종료: `-transfer-idle-timeout`, `-peer-wait-timeout`, `-seed-idle-timeout`으로 제어합니다.
- 후처리 커맨드: `-execute-on-complete`, `-execute-on-each-file-complete`와 `-command-timeout`으로 파이프라인 자동화.
- 관측성: `-verbose`(상세 로그), `-statistics`(전송 통계)로 병목을 파악하세요.

## CLI 레퍼런스

### 버전/로깅/통계
- `-version` (bool, 기본 false): 버전 출력 후 종료.
- `-verbose` (bool, 기본 false): 상세 로그 활성화. 성능/장애 분석에 유용.
- `-statistics` (bool, 기본 false): 완료 시 전송 통계 출력.
- `-help` (bool, 기본 false): 요약 사용법 출력.

### 대기/종료
- `-wait-host` (string): 대상 `host:port`의 `/completed`가 200을 반환할 때까지 대기.
- `-wait-host-timeout` (duration, 기본 10m): `-wait-host` 타임아웃.
- `-exit-complete` (bool, 기본 false): 모든 전송 완료 즉시 종료.

### 동시성/압축/청크/검증/후처리 커맨드
- `-peer-num-concurrent` (int, 기본 2, 1~16): 피어별 동시 전송 작업 수.
- `-local-num-concurrent` (int, 기본 1, 1~16): 로컬 동시 작업 수.
- `-compress-type` (string, 기본 `none`, {none,zstd}): 네트워크 압축.
- `-chunk-size` (int64, 기본 16777216=16MiB, 최대 134217728=128MiB): 전송 청크 크기(바이트 단위).
- `-files-per-chunk` (int, 기본 5): 청크당 최대 파일 수.
- `-verify-on-complete` (bool, 기본 false): 전체 완료 후 해시 검증 수행.
- `-execute-on-complete` (string): 전체 완료 시 실행할 커맨드. 템플릿: `%{elapsed_time}`, `%{file_count}`, `%{total_file_size}`.
- `-execute-on-each-file-complete` (string): 파일 완료 시 실행. 템플릿: `%{file_path}`, `%{file_size}`.
- `-command-timeout` (duration, 기본 10s): 후처리 커맨드 타임아웃.

### 네트워킹
- `-listen-addr` (string, 기본 `0.0.0.0:10090`): 서버 바인딩 주소.

### 디스커버리
- `-peer-list` (string): 정적 피어 목록 `host:port,host:port`. 동적 제공자와 병합됨.
- `-peer-list-url` (string): CSV(`host:port,host:port`)를 반환하는 HTTP 엔드포인트. 주기적 폴링. SRV/A와 상호 배타적.
- `-peer-list-srv` (string): DNS SRV 레코드 조회 (여러 피어 반환, 포트 정보 포함). 주기적 폴링. URL/A와 상호 배타적.
- `-peer-list-a` (string): DNS A 레코드 조회 (여러 IP 반환, 고정 포트 10090). 주기적 폴링. URL/SRV와 상호 배타적.
- `-peer-list-dns-server` (string): A/SRV 조회용 커스텀 DNS 서버 (예: 8.8.8.8:53).
- `-peer-list-poll-interval` (duration, 기본 3s): 동적 제공자(URL/SRV/A) 재조회 주기.
- `-peer-wait-timeout` (duration, 기본 20s): 최소 1개 피어 가용성 대기 타임아웃.

### 외부 레지스트리(Registry)

외부 레지스트리는 직접 구현해야 하는 서비스입니다. 피어가 자신을 등록하고, 다른 피어 목록을 조회하는 두 가지 엔드포인트를 제공해야 합니다.

**Heartbeat 등록 (POST)**

p2pcp는 `-peer-registry-heartbeat-url`로 지정된 엔드포인트에 주기적으로 JSON을 POST합니다:

```json
{
  "uuid": "123e4567-e89b-12d3-a456-426614174000",
  "address": "192.168.1.100:10090",
  "expires_in_seconds": 10
}
```

- `uuid`: 피어의 고유 식별자 (p2pcp가 자동 생성)
- `address`: 다른 피어가 접속할 수 있는 `host:port` (`-peer-registry-self-endpoint`로 지정)
- `expires_in_seconds`: 이 시간(초) 내에 다음 heartbeat가 없으면 레지스트리에서 제거해야 함. heartbeat 간격의 2배로 설정됨.

**피어 목록 조회 (GET)**

레지스트리는 별도의 GET 엔드포인트에서 활성 피어 목록을 CSV 형식으로 반환해야 합니다:

```
192.168.1.100:10090,192.168.1.101:10090,192.168.1.102:10090
```

이 엔드포인트 URL을 `-peer-list-url`에 지정하면 p2pcp가 주기적으로 폴링하여 피어 목록을 갱신합니다.

**CLI 옵션**
- `-peer-registry-heartbeat-url` (string): heartbeat를 POST할 엔드포인트 URL.
- `-peer-registry-self-endpoint` (string): 외부에서 이 피어에 접속 가능한 `host:port`.
- `-peer-registry-heartbeat-interval` (duration, 기본 5s): heartbeat 전송 주기.
- `-peer-registry-timeout` (duration, 기본 3s): 레지스트리 요청 타임아웃.

### 경로
- `-src` (string): 소스 경로(`local:/path` 또는 `/path`).
- `-dst` (string): 대상 경로(로컬 경로). 미지정 시 시드 모드.

### 타임아웃/동기화
- `-transfer-idle-timeout` (duration, 기본 30s, 0=무한대): 전송 유휴 타임아웃.
- `-seed-idle-timeout` (duration, 기본 0): 서버 유휴 종료 타임아웃(0은 비활성).
- `-sync-interval` (duration, 기본 2000ms): 피어 간 가용 청크 동기화 주기.

## 무결성 검증(해시)
- 기본 해시는 `xxh3`이며, HTTP `Digest` 헤더를 통해 검증합니다.
- `-verify-on-complete`로 전체 완료 후 해시 검증을 수행할 수 있습니다.

## 버전 호환성
- 피어 간 통신은 주요(major) 버전이 동일해야 합니다. 주요 버전이 다르면 전송을 거부합니다.

## HTTP 엔드포인트
- `/completed`: 완료 상태 확인
- `/uuid`: 서버 UUID (피어 자기 감지용)
- `/version`: 바이너리 버전 표시
- `/manifest`: 매니페스트 전달
- `/manifest/checksum`: 매니페스트 체크섬
- `/chunk`: 가용 청크 상태 (`only_updated_since` 쿼리 파라미터 지원)
- `/chunk/{index}`: 청크 데이터 전송 (압축 옵션 지원)

## 설치 / 빌드
- 요구 사항: Go 1.24+
```bash
make            # 정적 바이너리 (기본)
make dynamic    # 동적 바이너리 (시스템 NSS/DNS 연동이 필요한 환경용)
```

## 문제 해결(FAQ)
- 동적 디스커버리 옵션은 상호 배타적입니다. `-peer-list-url`/`-peer-list-srv`/`-peer-list-a` 중 하나만 지정하세요.
- 전송이 멈춘 것 같다면 `-transfer-idle-timeout`을 늘려 false negative를 줄여보세요.
- 처리량이 낮다면 동시성, 청크 크기, 압축 방식을 워크로드에 맞게 조정하세요.
- DNS 기반 디스커버리에 문제가 있다면 `-peer-list-dns-server`로 커스텀 DNS 서버를 지정해보세요.

## 라이선스
```
Copyright (c) 2023-present NAVER Corp.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
