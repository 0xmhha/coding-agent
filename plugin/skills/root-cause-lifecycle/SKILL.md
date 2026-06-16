---
name: root-cause-lifecycle
description: "버그 진단 추론 스캐폴드(diagnose/bugfix 전용). 틀린 값의 produce→store→consume 생애주기를 추적해 '어느 edge가 깨졌나'로 수렴한다. cks가 모은 후보 위에서 인과를 조립하는 L2→근본원인 다리. 설계측 거울상은 planner §5.2b(write-site completeness)."
type: skill
---

# Root-Cause Lifecycle Interrogation (진단 추론 — L2→근본원인 다리)

핵심 명제 (이 한 줄이 전부):
> **버그 = "소비자가 쓰는 값 == 생산자가 마지막으로 쓴 값" 불변식이 *어느 edge*에서 깨진 것.**
> 값의 모든 edge를 빠짐없이 열거하면 깨진 곳을 못 놓친다.

**언제**: "값이 틀리다 / stale 하다 / 변경이 반영 안 된다" 류 버그 진단(diagnose, bugfix 사이클).
**아닐 때**: 새 기능 설계(→ planner §5.2b), 프로토콜 모양·합의 안전성 판정(→ stablenet-invariants).
**관계**: cks를 대체하지 않는다. cks가 *후보 코드*를 주면, 이 절차가 *인과 사슬*로 조립한다.

## 절차 (이 순서를 강제한다)

1. **값 1개를 지목** — 증상이 *무엇에 관한* 것인가? 필드/슬롯/변수 단위로 구체적으로.
2. **생애주기를 그린다** — `produce → store/copy → consume`. **모든 복사본·캐시를 빠짐없이** 나열.
   cks 매핑: `find_callers`/`impact_analysis` = 복사본·소비자 열거, `change_history` = 생산자 변경 시점.
3. **edge별 실패모드를 열거** —
   - produce: 안 씀 / 잘못 씀 / 늦게 씀(타이밍)
   - store: **소스가 바뀌어도 갱신 안 됨(stale)** / 키 틀림 / 복사본이 갈라짐
   - consume: 잘못된 소스를 읽음 / stale 복사본을 읽음 / 비교 로직 오류 / 전제조건 틀림
4. **각 실패모드 → 함수 1개로 매핑** — 추측 말고 `file:line`. 없으면 cks로 찾는다.
5. ⭐ **증상의 구별특징으로 반증** — 예 "인상은 정상, 인하만 stuck" → 방향-의존 코드를 지목.
   그 특징을 *설명하지 못하는* 가설은 버린다.
6. ⭐ **소스까지 역추적** — stale 값을 찾아도 멈추지 마라. "이 값의 생산자는?"을 깨진 edge나
   생산자에 닿을 때까지 반복. **첫 캐시는 보통 *증상*이지 원인이 아니다.**
7. **캐시 무효화 점검** — 모든 캐시마다 "소스가 바뀔 때 무효화/갱신하는 코드가 있나?".
   **invalidator 0 인 캐시 = 유력 용의자.**

> ⭐ 5·6은 **검색·열거만으로는 안 나오는, 가장 자주 건너뛰는 두 동작**이다.
> 이게 빠지면 첫 stale 복사본에서 멈춰 진짜 source(상위 edge)를 놓친다 — 이것이 부분진단의 전형이다.

## 산출 (diagnosis.md / plan-fix-N.md 의 Root cause 절에 반영)

- 지목한 값 + 생애주기: producer / 복사본 목록 / consumer
- **깨진 edge 1개 + `file:line`**
- 반증으로 탈락시킨 경쟁 가설(왜 아닌지 한 줄씩)
- 확신도 + 무엇이 확신을 올리나(어떤 *판별변수*를 어떤 관측/재현이 보여주는가)

## 워크드 예 — PR77 (압축)

값 = effective minTip. 복사본 = ① state 슬롯 ② `AnzeonTipEnv.currentBlock` ③ `Transaction.anzeonTipCap` ④ `pool.gasTip`.
- 스텝7: ③ invalidator 0 → stale. **첫 캐시 = 증상.**
- 스텝6 역추적: ③의 소스 = ②(`GetAnzeonTipCap`). 스텝7을 ②에도 적용 → `SetCurrentBlock`이 root 동일이면 갱신 skip → **빈 블록에서 ②가 stale = 진짜 source.**
- 스텝5 반증: "인상 정상/인하 stuck" → `SetGasTip`의 `newTip>old` 분기를 지목.

→ ③에서 멈추면 부분진단. **5·6·7을 끝까지** 돌려야 진짜 원인(②)에 닿는다.

---
한 줄 트리거: **값 하나 잡고 → 모든 복사본 → 각 복사본에 무효화 있나 → 소스까지 역추적 → 증상 비대칭으로 반증.**
