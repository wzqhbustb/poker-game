# Poker AI 升级规划

## 1. 目标与结论

当前 Bot 是可运行、可解释的启发式策略，但还不是先进 Poker AI。核心限制是：

- 翻前用 Chen 分数和固定位置阈值，未维护 1326 组合级范围。
- 翻后权益按“随机对手手牌”计算，而非按行动不断收缩的对手范围。
- 动作通过固定权益阈值选择，而非比较 fold、call、不同下注尺度的期望收益（EV）。
- 缺少严格的 A/B 评测、置信区间、座位轮换和独立随机源。
- 缺少对手建模、状态抽象、离线训练和 CFR/MCCFR 求解能力。

升级目标不是简单增加规则或使用神经网络，而是建立一套**强度可测、决策可解释、训练可复现、线上可控**的 Poker AI 系统。

第一阶段边界：

- 9 人桌、100bb、NLHE 现金局，固定盲注 1/2。
- 暂不处理 rake、ante、straddle、锦标赛 ICM。
- 在线只做推理和轻量对手更新，训练放在线下。
- 不宣称完整 9 人桌 GTO；多人扑克不能直接套用双人零和 CFR 的理论保证。

## 2. 成功标准

先进程度必须用指标证明：

1. 永远输出合法动作，不读取不可见底牌，筹码严格守恒。
2. 对冻结旧策略及多种固定对手长期收益显著为正。
3. 所有收益报告包含 95% 置信区间，只有区间下界大于 0 才认定升级有效。
4. 范围预测、动作概率和弃牌概率可做校准测试。
5. CFR 在 Kuhn、Leduc 等可验证小型游戏上收敛到低 exploitability。
6. 在线决策满足 P95 100ms、硬截止 250ms，并有稳定回退策略。

## 3. 推荐总体架构

先把牌桌引擎与策略解耦，再逐层增加能力：

```text
engine ──> Observation ──> Policy
                            ├── LegacyPolicy
                            ├── RangeEVPolicy
                            ├── AdaptivePolicy
                            └── SolverPolicy

公开行动 ──> Range Tracker ──> Range Equity ──> Action EV ──> Mixed Strategy
                    ↑                                  │
              Opponent Model                    Rollout / Solver
```

建议逐步形成以下模块：

```text
server/internal/ai/
  policy.go          统一策略接口、决策结果和回退链
  observation.go     不可变的公开信息快照与派生特征
  range/             1326 组合、范围更新、阻断牌处理
  equity/            对范围权益、枚举、采样和缓存
  ev/                候选动作、对手响应和动作 EV
  opponent/          带先验和置信度的对手模型
  abstraction/       手牌、牌面和下注尺度抽象
  solver/            CFR/MCCFR 与小型验证游戏
  artifact/          策略制品版本、校验、加载和回退
server/cmd/eval/      配对 A/B、座位轮换、联赛评估
server/cmd/train/     离线训练、检查点和策略导出
```

运行时继续使用 Go。只有性能分析明确证明训练吞吐受限后，才考虑 C++、Python、GPU 或分布式训练。

## 4. 核心数据模型

### 4.1 Observation

策略输入必须是不可变且不含隐藏信息的观察对象：

- Hero 座位和底牌。
- 按钮、盲注、街道、公共牌。
- 每位玩家的公开状态：筹码、本街投入、总投入、fold/all-in/out。
- 当前底池、有效筹码、SPR、待跟额度。
- 完整公开行动历史和动作顺序。
- 合法动作、最小和最大 raise-to。
- 独立策略 RNG、决策截止时间、策略版本。

当前 `server/internal/bot/decide.go` 的 `DecisionView` 信息不足；后续策略不应直接接收 `*engine.Engine`。

### 4.2 ComboRange

- `ComboID`：稳定映射全部 1326 个两张牌组合。
- `ComboRange`：`[1326]float32` 权重。
- 每名未弃牌对手维护独立范围。
- 每次更新必须移除与 Hero/Board 冲突的组合并重新归一化。
- 169 类可用于展示，内部必须保留具体花色组合以正确处理 blocker。

### 4.3 DecisionResult

每次决策不仅返回动作，还应记录：

- 所有候选动作及混合概率。
- 各动作估计 EV、标准误和采样数。
- 范围、模型及策略制品版本。
- RNG seed、耗时、是否超时和回退原因。

EV 统一定义为“从当前决策点开始的净筹码变化”，以 fold EV = 0 为基准。

## 5. 分阶段实施路线

## 阶段 0：建立可信评估基线

**目的：先能准确判断新策略是否真的更强。**

工作内容：

1. 冻结当前策略为 `legacy-v1`，不再直接修改其参数。
2. 将 `server/cmd/sim/main.go` 泛化为可装载任意 `Policy` 的评估器。
3. 分离发牌 RNG、各策略动作 RNG、权益/rollout RNG。
4. 建立固定 deal set，确保相同策略版本和 seed 完全可复现。
5. 实施 duplicate evaluation：A/B 使用相同牌堆并交换、轮换座位。
6. 输出每手和每个 deal block 的收益、均值、标准差及 95% CI。
7. 建立策略池：Legacy、随机、Nit、Station、Maniac、过度弃牌等。

验收标准：

- 策略随机行为不会改变后续发牌。
- 所有策略获得相同的牌面和位置分布。
- 每手及整场净收益之和为零。
- 固定输入逐位可复现。
- 报告不再只提供单个 bb/100 点估计。

## 阶段 1：组合级范围与对范围权益

**目的：从“对随机牌算胜率”升级为“对合理范围算胜率”。**

工作内容：

1. 建立 1326 组合索引和 blocker-safe 范围运算。
2. 用版本化 chart 替换 Chen 核心，覆盖：
   - 各具体位置 RFI。
   - limp/iso。
   - 面对不同位置和尺度的 open。
   - cold call、squeeze、3-bet、4-bet、all-in。
3. 根据 `P(action | combo, context)` 贝叶斯式更新对手范围，而非简单截断。
4. 将 `EquityN` 扩展为对多个加权范围采样。
5. Chen 仅作为制品缺失时的回退策略。

验收标准：

- 范围非负、归一化、无冲突组合。
- 花色同构不改变结果。
- 各位置实际开池频率与 chart 目标误差不超过 1 个百分点。
- 均匀范围结果与旧随机权益估算在统计误差内一致。
- 常见权益点在 10k 以上采样时绝对误差约不超过 1%。

## 阶段 2：候选动作 EV

**目的：用收益比较取代固定权益阈值，这是近期最重要的强度跃迁。**

工作内容：

1. 生成有限且可控的候选动作：
   - fold/check/call。
   - 翻前标准 open、3-bet、4-bet、all-in。
   - 翻后约 25%、50%、75%、100%、150% pot 和 all-in。
2. 第一版构建单层 EV 模型：
   - 对手 fold/call/raise 概率。
   - 对继续范围的 showdown equity。
   - 当前成本、死钱、有效筹码和 fold equity。
3. 第二版加入有限深度 rollout 和统一叶节点价值函数。
4. 使用引擎快照或独立纯状态模拟器，不能修改在线牌局实例。
5. 根据 EV 生成混合策略，而非永远选择同一个最大值动作。
6. 建立回退链：完整 EV → 减少采样 → 查表策略 → LegacyPolicy。

验收标准：

- pot odds、必胜牌、零胜率牌、all-in 和边池场景的 EV 有解析测试。
- 所有候选动作最终通过引擎合法性校验。
- 在冻结 deal set 上进行至少数十万手配对评测。
- 对 `legacy-v1` 的 bb/100 差值 95% CI 下界大于 0 才晋级。
- 分位置、街道、SPR、单挑/多人池报告结果，避免总指标掩盖退化。

## 阶段 3：保守的对手建模

**目的：对明显偏离均衡的对手进行可控 exploit，同时避免小样本过拟合。**

工作内容：

1. 维护带先验的行为统计：RFI、3-bet、fold-to-3bet、各街 bet/call/raise/fold。
2. 按位置、人数和下注尺度 bucket 条件化。
3. 使用 Beta/Dirichlet 后验和 shrinkage，小样本回归总体先验。
4. 将动作 likelihood 用于范围更新和对手响应概率。
5. persona 仅作为测试对手，不再作为主策略的强度旋钮。
6. 线上只使用公开行动和已公开摊牌，禁止读取弃牌玩家未公开底牌。

验收标准：

- 在已知生成策略的数据上，Brier score/log loss 优于总体先验。
- 可靠性图不过度自信。
- 小于约 100 次相关观察时，策略偏移受到明确上限约束。
- 对 Station、Nit、Maniac 等固定对手显著增益。
- 对未知基准策略的退化不超过预设容忍值。

## 阶段 4：状态与动作抽象

**目的：控制后续求解器的状态空间，同时量化抽象损失。**

工作内容：

1. 提取牌面纹理：paired、monotone、连接度、高牌结构。
2. 提取私牌特征：成牌、有效 nuts、听牌、blocker、range equity。
3. 纳入位置、SPR、人数、行动序列和下注尺度。
4. 使用 equity histogram 或 potential-aware clustering 构建 hand bucket。
5. 对人类 off-tree 下注做最近尺度映射，并修正底池和后续 SPR。
6. 所有 abstraction schema 版本化。

验收标准：

- 花色同构状态映射至相同 bucket。
- bucket 内权益和未来潜力方差保持在设定阈值内。
- 动作映射始终合法。
- 在保留具体牌局上报告 abstraction loss。

## 阶段 5：CFR/MCCFR 求解

**目的：建立经理论验证的求解能力，而不是直接挑战完整 9 人全树。**

实施顺序：

1. 实现 Kuhn Poker，验证 regret matching 和平均策略。
2. 实现 Leduc Poker，加入检查点、恢复和 deterministic seed。
3. 实现 heads-up river、固定牌面、有限下注尺度子游戏。
4. 精确遍历无法扩展后，引入 external-sampling MCCFR。
5. 扩展到 heads-up postflop 子游戏或受限 100bb blueprint。
6. 9 人桌采用混合方案：
   - 翻前 chart/blueprint。
   - 多人池 Range EV。
   - 进入 heads-up 后使用更强求解策略。
   - 多人复杂场景使用策略池训练和安全回退。

验收标准：

- Kuhn/Leduc 平均策略接近已知均衡。
- exploitability/NashConv 随迭代稳定下降。
- MCCFR 与精确 CFR 在同一小型游戏上得到近似结果。
- 中断恢复与连续训练结果等价。
- 不用“训练 loss 下降”替代真实收益和 exploitability 验证。

## 阶段 6：训练与发布产品化

离线训练：

- `cmd/train` 固定游戏配置、抽象版本和随机种子。
- 支持周期检查点和中断恢复。
- 候选制品自动参加冻结策略池联赛。
- 训练、调参、最终测试的 deal seeds 严格隔离。
- 制品携带 schema hash、代码版本、训练指标和校验和。

在线推理：

- 启动时加载并校验只读制品。
- 每桌独立维护范围和对手后验，不在线更新全局策略权重。
- 记录策略版本、动作概率、EV、耗时和回退原因。
- 先 shadow mode 计算但不执行，再逐步启用。
- 保留即时切回 LegacyPolicy 的能力。

## 6. 性能预算

初始在线目标：

| 环节 | 目标 |
|---|---:|
| 范围过滤与更新 | P95 ≤ 2ms |
| 候选动作生成 | P95 ≤ 1ms |
| 权益与有限 rollout | 约 80ms 预算 |
| 总决策时间 | P50 ≤ 25ms，P95 ≤ 100ms |
| 硬截止 | 250ms |
| 单桌动态 AI 状态 | < 2MB |
| 首版策略制品 | < 256MB |

人为思考延迟必须和真实计算时间分离。优化顺序是减少内存分配、牌力缓存、范围采样优化和花色同构；未经过 profiler 证明前不引入 GPU。

## 7. 评测纪律

为防止“看起来赢了，实际是方差或调参污染”，必须遵守：

1. A/B 使用共同随机牌堆。
2. 完整轮换按钮、盲位和所有对手座位。
3. 发牌、策略动作和 rollout 使用独立 RNG。
4. 按 deal block 计算置信区间，不能把相关重放当独立样本。
5. 冻结基线、策略池和最终测试集。
6. 同时测试多类对手，避免剪刀石头布式虚假冠军。
7. 报告位置、街道、SPR、人数、all-in、摊牌和非摊牌分层指标。
8. 预设样本量或顺序检验规则，禁止“刚好赢时停止”。
9. 多参数并行试验要保留独立最终测试集。
10. 保存最大负 EV 决策，支持确定性 replay 和人工审查。

## 8. 不应过早做的事情

- 不直接实现完整 9 人、100bb、连续下注空间的全树 CFR。
- 不把多人自博弈结果宣传成严格 GTO。
- 不先上神经网络、深度强化学习、GPU 集群或分布式训练。
- 不根据几十手真人数据激进调整范围。
- 不用真人短期盈亏作为训练目标。
- 不使用单一阵容、单一 seed 或单个 bb/100 选择冠军。
- 不先引入几十种下注尺度。
- 不在一个版本同时替换范围、EV、抽象和求解器。
- 不让 LLM 直接决定实时扑克动作；LLM 只保留为复盘教练。

## 9. 推荐优先级与里程碑

| 优先级 | 里程碑 | 主要产物 | 晋级条件 |
|---|---|---|---|
| P0 | 可信评估 | Policy 接口、固定牌局、配对 A/B、95% CI | 可复现且零和守恒 |
| P0 | 范围系统 | ComboRange、chart、范围更新、范围权益 | blocker/归一化/权益测试通过 |
| P0 | 动作 EV | 候选动作、一层 EV、回退链 | 显著战胜 legacy-v1 |
| P1 | 对手模型 | 贝叶斯统计、校准报告 | 预测优于总体先验且不过拟合 |
| P1 | 抽象系统 | hand/board/action abstraction | 抽象损失处于阈值内 |
| P2 | CFR/MCCFR | 小型游戏、子游戏求解器 | exploitability 稳定下降 |
| P2 | 制品发布 | train/eval、联赛、shadow mode | 质量和延迟门禁全部通过 |

## 10. 下一迭代建议

第一轮只做以下四项，不同时启动 CFR：

1. 定义 `Policy`、`Observation`、`DecisionResult`。
2. 将现有 Bot 包装为冻结的 `LegacyPolicy`。
3. 将 `cmd/sim` 改造成固定牌局、RNG 隔离、座位轮换的 A/B 评估器。
4. 建立首个 1326 组合级 `ComboRange` 及 blocker-safe 测试。

完成这些基础后，再实现“对范围权益”和“一层动作 EV”。这是从当前启发式 Bot 向先进 Poker AI 演进中投入产出最高、风险最低的路径。

## 11. 当前关键文件

- `server/internal/bot/decide.go`：当前翻前/翻后决策核心。
- `server/internal/bot/range.go`：Chen 分数和位置阈值。
- `server/internal/bot/equity.go`：对随机范围的蒙特卡洛权益。
- `server/internal/engine/engine.go`：牌局状态和可观测信息来源。
- `server/internal/engine/game.go`：动作执行、下注轮和结算。
- `server/cmd/sim/main.go`：现有自博弈基准入口。
- `server/internal/table/table.go`：在线策略调用和牌桌编排。
- `server/internal/store/store.go`：历史数据与未来决策追踪的持久化基础。
