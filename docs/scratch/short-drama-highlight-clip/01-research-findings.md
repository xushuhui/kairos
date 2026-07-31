# ASR + LLM 供应商技术选型调研报告

> 调研目标：为规划中的独立 Go 微服务选型 ASR（语音转文字）与 LLM（短剧高光片段语义判定）供应商组合。
> 场景：中文短剧单集视频（1-3分钟），需转写台词（含口语化表达、背景音乐/音效干扰）并用 LLM 判定最精彩的连续1分钟片段。
> 调研日期：2026-07-23。本文档为纯调研结论，不含代码，不做架构决策。
> 方法论：优先引用官方文档/定价页/API参考/官方技术报告；其次引用学术论文/独立评测；营销类表述均标注"宣传性质"；无法找到权威来源的结论一律标注 **【推测】**。

---

## 一、ASR 候选逐一分析

### 1.1 阶跃云（StepFun）ASR — `stepaudio-2.5-asr`

| 维度 | 结论 |
|---|---|
| **中文准确率** | 官方技术报告（arXiv:2605.23463）自测：中文开源测试集平均 **CER 2.97%**（AISHELL-1 低至0.71%，FLEURS-zh 2.63%），长音频测试集平均错误率3.70%。评测场景为"新闻/会议/强噪声"，**短剧口语化+BGM场景无专项数据【推测，未找到官方来源】**，需自测验证。 |
| **分句/分词时间戳** | ✅ 原生支持。异步文件识别接口 `/v1/audio/asr/file/submit`（`show_utterances=true`）返回**分句级**`start_time`/`end_time`（ms）+ 逐句 `words[]` **字/词级**时间戳，另支持说话人分离。同步简易转写接口 `/v1/audio/transcriptions` **无时间戳**，需用异步接口。来源：[API参考-ASR](https://platform.stepfun.com/docs/zh/api-reference/audio/asr) |
| **异步/批量与延迟** | ✅ submit+query 异步模式；引擎侧 RTF≈0.0053（1小时音频约19秒推理），5分钟音频可在1秒内出结果（不含排队/网络延迟，无官方端到端SLA）。来源：[模型页](https://platform.stepfun.com/docs/zh/guides/models/stepaudio-2.5-asr) |
| **计价** | **0.15 元/小时 ≈ ¥0.0025/分钟**，是本次调研中**最便宜**的ASR选项（约为上代 step-asr 的1/10）。来源：[定价页](https://platform.stepfun.com/docs/zh/guides/pricing/details) |
| **Go SDK/REST** | 无官方Go SDK（GitHub组织 stepfun-ai 无Go SDK仓库）。REST接口简单：`Authorization: Bearer $API_KEY`，无AK/SK签名，Go标准库 `net/http` 可直接对接。来源：[stepfun-ai GitHub](https://github.com/orgs/stepfun-ai/repositories) |
| **合规** | 用户协议第2.1.3条要求用户自证版权合法性（平台不背书）；第5.3条用户授予平台"提升大模型服务体验"的使用许可，**授权范围较宽泛，未见明确"不用于训练"承诺**——是本次调研中合规条款相对最弱的一项。来源：[用户协议](https://platform.stepfun.com/docs/zh/agreement/userservice.md)、[隐私政策](https://platform.stepfun.com/legal/privacy-policy.html) |

### 1.2 OpenAI Whisper API（`whisper-1`）

| 维度 | 结论 |
|---|---|
| **中文准确率** | 官方未披露中文专项WER。第三方学术评测：Whale论文（[arXiv:2506.01439](https://arxiv.org/pdf/2506.01439)）Common Voice中文 **CER≈12.8%**；DOTA-ME-CS口语化数据集论文（[arXiv:2501.12122](https://arxiv.org/pdf/2501.12122)）**WER 17.7%/CER 17.6%，且明显落后于Paraformer/SenseVoice等国产模型**。中文能力在同类中**排名靠后**。 |
| **分句/分词时间戳** | ⚠️ **仅 `whisper-1` 支持** `timestamp_granularities[]`（word/segment级，需配合`verbose_json`）；新模型 `gpt-4o-transcribe`/`mini` **不支持**该参数，只能拿纯文本。若选OpenAI必须用 whisper-1。来源：[Speech-to-text指南](https://platform.openai.com/docs/guides/speech-to-text) |
| **异步/批量与延迟** | ❌ **不支持批量任务**——官方 Batch API 端点列表**不含** `/v1/audio/transcriptions`。只能同步HTTP调用，单文件≤25MB（1-3分钟短剧远小于此限制，无影响）。`whisper-1` 不支持流式。来源：[Batch指南](https://platform.openai.com/docs/guides/batch) |
| **计价** | **$0.006/分钟**（whisper-1）≈ ¥0.043/分钟（按7.2汇率）。来源：[模型页](https://platform.openai.com/docs/models/whisper-1) |
| **Go SDK/REST** | ✅ 官方Go SDK `github.com/openai/openai-go` 原生覆盖 Audio Transcriptions 端点，是本次调研**Go集成体验最好**的供应商之一（Bearer Token，无签名）。 |
| **合规** | ✅ 官方明确：API数据默认**不用于训练**（2023年3月起）；音频转写端点（`/v1/audio/transcriptions`）数据保留策略是**全端点中最严格**的（训练用途：否；滥用监控日志保留：None）。来源：[数据控制页](https://platform.openai.com/docs/guides/your-data)。⚠️ 但**音频端点数据仅支持美国/欧盟区域处理，无中国大陆节点**，短剧内容跨境处理需自评合规风险；版权上传无专门条款，责任在客户（服务条款不为未授权内容侵权提供赔偿）。 |

### 1.3 华为云语音识别（SIS，Speech Interaction Service）

| 维度 | 结论 |
|---|---|
| **中文准确率** | 官方仅宣传"准确率超过95%"（[产品专题页](https://www.huaweicloud.com/theme/242359-1-Y)），**营销性质，无第三方评测数据**，短剧场景无专项数据【推测】。 |
| **分句/分词时间戳** | ✅ 原生支持。同步极速版（`RecognizeFlashAsr`，`need_word_info=yes`）与异步长语音（`PushTranscriberJobs`/`CollectTranscriberJob`）均返回句级`start_time`/`end_time`（ms）+ `word_info[]`字/词级时间戳，异步接口另可选返回角色/情绪/语速（仅8k模型）。来源：[API文档](https://support.huaweicloud.com/api-sis/sis_03_0092.html) |
| **异步/批量与延迟** | ✅ 异步任务：<10分钟音频典型延迟**<2分钟**；同步极速版无需轮询但官方未给出具体延迟数字【推测】。⚠️ **服务仅支持"华北-北京四"/"华东-上海一"两个区域**，需确认与主体业务的区域匹配。来源：[约束与限制](https://support.huaweicloud.com/productdesc-sis/sis_01_0018.html) |
| **计价** | 异步版 **¥2.50/小时 ≈ ¥0.042/分钟**；极速版 **¥3.00/小时 ≈ ¥0.05/分钟**（2026-07-23官方价格计算器实测）。来源：[价格计算器](https://www.huaweicloud.com/pricing/calculator.html#/sis) |
| **Go SDK/REST** | ✅ 官方Go SDK `github.com/huaweicloud/huaweicloud-sdk-go-v3/services/sis/v1`，已直接确认源码含 `RecognizeFlashAsr`/`PushTranscriberJobs`/`CollectTranscriberJob` 三个对应方法，AK/SK签名由SDK自动完成。fm-kafka 已在用同一 SDK 家族（`pkg/hwvod`），凭据管理经验可复用。 |
| **合规** | 无SIS专属条款，适用通用《华为云用户协议》：版权责任在用户（1.5条），平台承诺"不会访问或使用您的内容，除非为提供服务所必要"（未列出训练用途，**【推测】默认不用于训练**，非明确书面声明）。识别结果72小时后过期，需自行落盘。来源：[用户协议](https://www.huaweicloud.com/declaration/sa_cua_computing.html) |

### 1.4 腾讯云语音识别（ASR）

| 维度 | 结论 |
|---|---|
| **中文准确率** | ✅ 本次调研**唯一有第三方检测机构背书**的数字：国家电子计算机质量监督检验中心测试报告，干净16k/16bit音频下**中文字准率97.40%**（官方明确注明"仅供参考，不作为服务准确性承诺"）。来源：[FAQ](https://cloud.tencent.com/document/product/1093/35802)。营销页宣称的98%/99.9%等数字**未经验证，标记推测**。短剧口语化+BGM场景无专项数据。 |
| **分句/分词时间戳** | ✅ 原生支持。标准版异步接口 `ResultDetail[]` 含句级`StartMs`/`EndMs` + `Words[]`词级`OffsetStartMs`/`OffsetEndMs`；极速版 `word_info` 参数（0-3档）可选词级/标点分段时间戳。均已核实官方真实JSON示例。来源：[标准版结果查询](https://cloud.tencent.com/document/product/1093/37822)、[极速版](https://cloud.tencent.com/document/product/1093/52097) |
| **异步/批量与延迟** | ✅ 标准版异步：官方明确"最长3小时返回，大多数1小时音频1-3分钟完成"；极速版同步：官方明确"30分钟音频可在10秒内完成"。来源：[计费概述](https://cloud.tencent.com/document/product/1093/35686) |
| **计价** | 标准版后付费阶梯：基础引擎 **¥1.75/小时**（低量档，≈¥0.029/分钟）到 **¥0.80/小时（大模型2.0版，统一价，≈¥0.013/分钟）**；极速版 **¥3.10/小时起 ≈ ¥0.052/分钟**。来源：[计费概述](https://cloud.tencent.com/document/product/1093/35686) |
| **Go SDK/REST** | ✅ 官方Go SDK `github.com/tencentcloud/tencentcloud-sdk-go`（`tencentcloud/asr/v20190614`），已核实覆盖 `CreateRecTask`/`DescribeTaskStatus`，TC3-HMAC-SHA256签名由SDK自动处理，是本次调研中**Go工程接入体验最成熟**的选项之一（另有专用 `tencentcloud-speech-sdk-go` 覆盖极速版）。 |
| **合规** | 《语音识别服务条款》第2.6条：版权责任在用户（平台不预审）。音频**"仅供当次识别使用，不会进行保存"**，文本结果保存7天。⚠️ 国际站《AI Service Terms》明确"未经opt-in不会用于训练AI模型"，但**该条款出自国际站，未找到中国大陆站的完全对应条款【推测，建议签约前书面确认】**。来源：[ASR服务条款](https://cloud.tencent.com/document/product/301/94121)、[国际站条款](https://www.tencentcloud.com/document/product/301/74049) |

### 1.5 阿里云智能语音交互（ISI + DashScope Paraformer 两条产品线）

| 维度 | ISI（智能语音交互，老牌） | DashScope Paraformer-v2（百炼，主推新入口） |
|---|---|---|
| **中文准确率** | 官方宣传"90%+/98%"（[产品页](https://www.aliyun.com/product/ai/nls/asr)），CNAS认证测试>98%（**理想实验室条件**，非真实场景） | ✅ **有公开学术论文**：Paraformer原论文（[arXiv:2206.08317](https://arxiv.org/abs/2206.08317)）AISHELL-1 CER **5.2%**；Paraformer-v2论文（[arXiv:2409.17746](https://arxiv.org/abs/2409.17746)）明确以"noise-robust"（抗噪）为改进目标，理论上更适配短剧BGM场景，但无中文噪声场景量化数字 |
| **分句/分词时间戳** | ✅ 支持，句级默认返回，词级需 `enable_words=true` | ✅ **默认同时输出句级+字级两种粒度**，无需额外开关，比ISI更省心 |
| **异步/批量与延迟** | ✅ 异步；免费用户24h内、**付费用户3小时内**完成，结果保留72小时 | ✅ 异步（`X-DashScope-Async: enable`）；模型RTF≈0.009，队列延迟"通常几分钟"（无硬SLA），结果链接24小时有效 |
| **计价** | 标准版 **2.50元/小时起 ≈ ¥0.042/分钟**（阶梯至1.00元/小时） | ✅ **0.00008元/秒 = 0.288元/小时 ≈ ¥0.0048/分钟**——**本次调研云厂商中最便宜的**（约为ISI的1/8.7），每月免费额度10小时 |
| **Go SDK/REST** | ✅ 官方Go SDK `alibabacloud-nls-go-sdk`，但**不覆盖录音文件识别**；文件识别走通用 `alibaba-cloud-sdk-go`（AK/SK RPC签名），官方有Go示例 | ❌ 官方仅提供Python/Java SDK，**无官方Go SDK**；REST接口简单（`Authorization: Bearer`，无AK/SK签名），签名复杂度低于ISI |
| **合规** | 《智能语音交互服务协议》：版权责任在用户；默认不用于训练，仅签署《用户授权书》场景例外 | ✅ **官方隐私声明明确**："阿里云严格保护数据隐私，**绝不会将您的数据用于模型训练**"，且已通过SOC 2审计——本次调研中**合规条款表述最明确**的供应商之一。来源：[隐私声明](https://help.aliyun.com/zh/model-studio/privacy-notice) |

来源：[ISI产品文档](https://help.aliyun.com/zh/isi/developer-reference/api-reference-2)、[ISI计费](https://help.aliyun.com/zh/isi/product-overview/billing-10)、[DashScope录音文件识别](https://help.aliyun.com/zh/model-studio/paraformer-recorded-speech-recognition-restful-api)、[DashScope计费](https://help.aliyun.com/zh/isi/developer-reference/metering-and-billing)

### 1.6 讯飞开放平台（iFlytek）ASR

| 维度 | 结论 |
|---|---|
| **中文准确率** | 官方仅营销口径"98%"/自营销博客声称"3.2%字错率"（**均为宣传性质，非独立评测，可信度存疑**）。未找到权威第三方WER/CER数据。 |
| **分句/分词时间戳** | ✅ 原生支持，三条产品线（标准版/极速版/大模型版）均返回句级`bg`/`ed`（ms）+ 词级`wordBg`/`wordEd`（10ms帧）时间戳，含词属性（人名/数字/语气词等标注）。来源：[标准版API](https://www.xfyun.cn/doc/asr/lfasr/API.html)、[大模型版API](https://www.xfyun.cn/doc/spark/asr_llm/Ifasr_llm.html) |
| **异步/批量与延迟** | ✅ 三条产品线均为异步。极速版/大模型版延迟最优：**1小时音频约1分钟返回**；标准版SLA承诺最长5小时。 |
| **计价** | ⚠️ **仅支持预付费"时长套餐"，无按量后付费模式**。最新"录音文件转写大模型版"性价比最高：**低至¥0.02/分钟**（30万小时档），入门套餐（80小时/¥198）约¥0.041/分钟。来源：[产品定价页](https://www.xfyun.cn/services/lfasr) |
| **Go SDK/REST** | 无官方Go SDK（仅Java SDK）。REST可用但**三条产品线签名算法互不统一**（HMAC-SHA1/HMAC-SHA256混合），极速版签名最复杂（需拼接host/date/request-line/digest），需自行用Go `crypto/hmac`实现，工程量中等偏高。 |
| **合规** | ⚠️ **本次调研中唯一在隐私政策中正面声明会用（匿名化/去标识化后的）语音数据训练模型**的供应商："…包括使用匿名化或去标识化后的语音信息进行模型算法的训练"。来源：[隐私政策](https://www.xfyun.cn/doc/policy/privacy.html)。用户协议同时保留将上传内容用于"广告营销/技术优化"的权利。版权合法性责任仍在用户。 |

### ASR 横向对比速览

| 供应商 | 每分钟成本(¥) | 时间戳能力 | 官方Go SDK | 合规明确度 |
|---|---|---|---|---|
| 阿里云 DashScope Paraformer-v2 | **¥0.0048**（最低） | ✅ 默认句+字级 | ❌（REST简单） | ✅✅ 明确"绝不用于训练" |
| 阶跃云 stepaudio-2.5-asr | ¥0.0025（理论最低，无学术验证准确率） | ✅ 句+字/词级 | ❌（REST简单） | ⚠️ 授权范围宽泛，无明确承诺 |
| 腾讯云（大模型2.0引擎） | ¥0.013 | ✅ 句+词级 | ✅✅（最完整） | ⚠️ 国际站有承诺，大陆站未confirm |
| 华为云 SIS | ¥0.042 | ✅ 句+字级 | ✅ | ⚠️ 推断默认不训练，非明示 |
| 讯飞大模型版 | ¥0.02（预付费套餐制） | ✅ 句+词级 | ❌（签名复杂） | ❌ 唯一明示会用于训练 |
| OpenAI whisper-1 | ¥0.043 | ✅ 唯一支持的模型 | ✅✅ | ✅ 明确不训练，但无中国节点 |

---

## 二、LLM 候选逐一分析（短剧转写文本高光片段判定任务）

> 场景说明：短剧单集转写文本量通常仅数百至数千字（对应数千token），远低于所有候选的上下文上限；因此**上下文窗口大小对当前场景是冗余能力**，重点应放在中文理解质量、成本、Go集成便利性上。所有候选均**无针对"短剧剧情理解/高光片段判定"的公开专项评测**，这是本次调研在LLM侧最大的证据缺口。

### 2.1 阶跃云 Step 系列（`step-3.5-flash`）

- **上下文**：256K tokens。来源：[模型页](https://platform.stepfun.com/docs/zh/guides/models/step-3.5-flash)
- **中文理解**：官方技术报告（[arXiv:2602.10604](https://arxiv.org/abs/2602.10604)）C-Eval 89.6、CMMLU 88.9（5-shot，**厂商自测，二手转述**）；Artificial Analysis第三方智能指数评测中"智能-速度帕累托前沿排名第一"（[来源](https://x.com/ArtificialAnlys/status/2062381047212638697)），属通用智能评测，非中文/剧情专项。
- **成本**：¥0.7/¥2.1 每百万输入/输出token。来源：[定价页](https://platform.stepfun.com/docs/zh/guides/pricing/details)
- **Go/OpenAI兼容**：无官方Go SDK；✅ **官方明确OpenAI API协议完全兼容**，可直接用OpenAI官方Go SDK换`base_url`调用。来源：[迁移指南](https://platform.stepfun.com/docs/zh/guides/developer/openai)
- 注：任务背景提及的 `step-2`/`step-1.5`/`step-r1` 均为已下线/非现役历史型号，选型应直接使用 `step-3.5-flash`。

### 2.2 DeepSeek（`deepseek-v4-flash` / `deepseek-v4-pro`）

- **上下文**：**1,000,000 tokens（1M）**。来源：[定价页](https://api-docs.deepseek.com/quick_start/pricing/)
- **中文理解**：✅ 本次调研**中文基准分数最高**——官方技术报告（[arXiv:2606.19348](https://arxiv.org/abs/2606.19348)）C-Eval **92.1(flash)/93.1(pro)**，CMMLU **90.4/90.8**，LongBench-V2长文本理解 **44.7/51.5**（较上代V3.2的40.2显著提升）。
- **成本**：✅ **本次调研最便宜的LLM**——V4-flash 输入 $0.14/M（缓存命中仅$0.0028/M）、输出$0.28/M；V4-pro输入$0.435/M、输出$0.87/M。支持前缀缓存自动折扣。来源：[定价页](https://api-docs.deepseek.com/quick_start/pricing/)
- **Go/OpenAI兼容**：无官方Go SDK（GitHub组织无客户端SDK仓库）；✅ 官方明确兼容OpenAI/Anthropic API格式，可直接用OpenAI官方Go SDK换`base_url`（`https://api.deepseek.com`）调用。fm-kafka 已有 `utils/deepseek.go` 使用 `sashabaranov/go-openai` 客户端的先例，但目前指向本地 ollama（`localhost:11434`），新服务需改指向真实 DeepSeek 云端点，不能直接复用现有连接配置。
- ⚠️ **注意**：旧模型名 `deepseek-chat`/`deepseek-reasoner` 已于2026-07-24（本报告调研次日）下线，选型须直接使用新模型名。

### 2.3 通义千问（Qwen，DashScope/百炼）

- **上下文**：主力档 `qwen3.6-flash`/`qwen3.7-plus`/`qwen3.7-max` 均为**1M**；专用长文档模型`qwen-long`达10M。来源：[模型清单](https://www.alibabacloud.com/help/en/model-studio/text-generation-model/)
- **中文理解**：最新一代官方未单独公布C-Eval/CMMLU（**推测因分数已饱和不再单测**）；旧代Qwen2.5-72B C-Eval≈81.8（二手聚合数据，需谨慎）；独立LMArena快讯（2026-07-20）`qwen3.7-max-preview`综合排名第18（ELO 1475）。
- **成本**：`qwen3.6-flash` **¥1.2/¥7.2** 每百万输入/输出token；`qwen-long` 低至¥0.5/¥2。来源：[定价页](https://help.aliyun.com/zh/model-studio/model-pricing)
- **Go/OpenAI兼容**：无官方原生Go SDK（仅Python/Java）；✅ 官方**文档化推荐**使用OpenAI官方Go SDK + 百炼OpenAI兼容Base URL，Bearer Token鉴权简单。

### 2.4 月之暗面 Kimi（Moonshot AI，K2.6/K3）

- **上下文**：K2.6=262,144（256K）；K3=1,048,576（约1M）。来源：[定价页](https://platform.kimi.com/docs/pricing/chat-k26)
- **中文理解**：官方技术报告未发布C-Eval/CMMLU；✅ 独立LMArena快讯中**综合排名第9（ELO 1486）**，是本次三家中文LLM候选中**Arena排名最高**的（高于Qwen的#18、GLM的#27-30），且在Code Arena前端代码榜排名全球第一（二手newsletter信息，需谨慎对待）。
- **成本**：K2.6 **¥6.5/¥27**（缓存未命中输入/输出）每百万token；K3因始终启用推理链，输出单价高达¥100/M，**成本显著高于DeepSeek/Qwen**。来源：[K2.6定价](https://platform.kimi.com/docs/pricing/chat-k26)
- **Go/OpenAI兼容**：无官方Chat Completion Go SDK（仅有面向本地CLI Agent的`kimi-agent-sdk`，不适用于后端服务）；✅ 官方声明OpenAI API格式兼容，可用OpenAI Go SDK+自定义base_url。

### 2.5 智谱AI GLM（`glm-5.2`）

- **上下文**：官方称1M（[模型卡](https://docs.bigmodel.cn/cn/guide/models/text/glm-5.2)），但经阿里云百炼转发时第三方页面显示为198K，**口径不一致，需按实际接入路径核实**。
- **中文理解**：GLM-5.2官方评测聚焦代码/Agent能力，未见C-Eval/CMMLU；旧代GLM-4.5技术报告（[arXiv:2508.06471](https://arxiv.org/pdf/2508.06471)）C-Eval 86.9；独立LMArena排名在三家中**最低**（glm-5.2 #30，ELO 1468）。
- **成本**：GLM-5.2 **¥8/¥28** 每百万输入/输出token，是三家中**价格最高**的（定价数据来自论坛二手转述，非官方页面直接抓取，需上线前核实）。
- **Go/OpenAI兼容**：无官方Go SDK（仅Python/Java），官方推荐OpenAI兼容endpoint。

### LLM 横向对比速览

| 供应商/模型 | 上下文 | 中文基准(C-Eval/CMMLU) | 每百万token成本(输入/输出,¥或$) | Arena排名(2026-07-20快讯) | Go集成 |
|---|---|---|---|---|---|
| **DeepSeek V4-flash** | 1M | **92.1 / 90.4**（官方技术报告） | $0.14 / $0.28（最低） | 未收录 | OpenAI兼容 |
| 通义千问 qwen3.6-flash | 1M | 未公布(推测饱和) | ¥1.2 / ¥7.2 | #18 (ELO 1475) | OpenAI兼容(官方推荐) |
| Kimi K2.6 | 256K | 未公布 | ¥6.5 / ¥27 | **#9 (ELO 1486，最高)** | OpenAI兼容 |
| 阶跃云 step-3.5-flash | 256K | 89.6 / 88.9（官方技术报告） | ¥0.7 / ¥2.1 | 未收录 | OpenAI兼容 |
| 智谱GLM-5.2 | 1M(或198K) | 未公布(旧代86.9) | ¥8 / ¥28（最高） | #27-30(最低) | OpenAI兼容 |

---

## 三、推荐组合与理由

### 推荐主方案：**腾讯云语音识别（大模型2.0引擎，录音文件识别标准版）+ DeepSeek V4-flash**

**ASR选腾讯云的理由：**
1. 本次调研中**唯一有第三方检测机构（国家电子计算机质量监督检验中心）背书的准确率数字**（97.40%字准率），虽为理想条件测试，但相比其他厂商纯营销口径的"98%/99.9%"更可信。
2. **官方Go SDK完整覆盖**所需接口（`CreateRecTask`/`DescribeTaskStatus`），TC3-HMAC-SHA256签名由SDK自动处理，是六个候选中Go工程接入最成熟稳妥的（同时兼顾另有专用SDK覆盖极速版，未来需要更低延迟时可平滑切换）。
3. 时间戳能力（句级+词级，ms精度）、异步任务延迟（1小时音频1-3分钟出结果）完全满足需求。
4. 成本可控（大模型2.0引擎 ¥0.013/分钟），且音频原文"不留存，仅当次识别使用"，从隐私角度对上传短剧素材相对友好。

**LLM选DeepSeek V4-flash的理由：**
1. 本次调研**中文理解基准分数最高**（C-Eval 92.1、CMMLU 90.4，均高于Qwen/Kimi/GLM有据可查的同期数据），长文本理解基准LongBench-V2达44.7，对"理解完整台词文本+精准定位片段"这类需要抓取长文本细节的任务有正向支撑。
2. **成本最低**（输入$0.14/M、输出$0.28/M，命中缓存后几乎可忽略），短剧场景单集调用成本可忽略不计。
3. 1M超长上下文为未来扩展（多集合并分析、全剧本级判定）预留充足空间。
4. 官方明确OpenAI API兼容，Go侧可直接用OpenAI官方Go SDK换`base_url`接入，与ASR选型的工程模式（REST/SDK）保持一致，学习和维护成本低。

### 备选方案（按不同优先级排序）

- **极致成本优先** → 阶跃云 stepaudio-2.5-asr（¥0.0025/分钟）+ 阶跃云 step-3.5-flash 或 DeepSeek V4-flash：ASR+LLM 单账单管理更简单（若选阶跃云LLM），但阶跃云ASR合规条款是六者中相对最弱的一项，仅推荐用于已获充分授权、风险可控的自制内容。
- **合规优先** → 阿里云 DashScope Paraformer-v2（官方隐私声明明确"绝不用于训练"，价格¥0.0048/分钟，是云厂商中付费ASR最便宜的）+ 通义千问 qwen3.6-flash 或 DeepSeek V4-flash：适合对"不用于训练"这一条款有硬性合规要求的团队，且Paraformer默认同时输出句/字级时间戳（比其他厂商更省心）。
- **Go工程一致性优先** → 华为云 SIS（官方Go SDK已核实覆盖全部所需方法）+ DeepSeek V4-flash：华为云价格适中（¥0.042/分钟）、区域较少（仅两个区域）是主要限制。

### 主要风险与不确定性

1. **【核心证据缺口】短剧口语化+背景音乐场景下的真实ASR准确率，六家候选均无公开专项评测数据**——包括推荐的腾讯云在内，97.40%数字是干净16k/16bit朗读音频下的第三方检测结果，不代表含BGM/音效的短剧真实台词场景表现。**强烈建议选型定案前，用20-50条真实短剧素材对腾讯云/阿里云DashScope/阶跃云做小规模POC横向实测（WER人工核对），再最终决定。**
2. **【核心证据缺口】"中文短剧剧情理解/高光片段判定"这一具体任务，五个LLM候选均无公开评测或案例**——所有判断只能依赖C-Eval/CMMLU等通用中文理解基准与LongBench等长文本基准做间接推断，**不能保证DeepSeek V4-flash在"判定最精彩1分钟"这一主观性较强的任务上实际表现最优**。建议用10-20个已知"精彩片段"的短剧样本，对DeepSeek/Qwen/Kimi三家做人工评估的小规模盲测，再定最终LLM。
3. **DeepSeek V4是2026年4月才发布的Preview系列**，模型命名/定价短期内已发生过变更（V3.2→V4，旧模型名两个月内下线），需持续关注官方定价页与模型下线公告，避免因命名变更导致服务中断。
4. **版权内容上传合规是全行业共性风险，非某一供应商独有**：六家ASR厂商的服务条款中，**没有一家对"上传受版权保护的视频/音频内容做转写"给出正面明示许可**，全部条款采用"用户自行确保内容合法性/已获授权，责任在用户方"的中立技术服务商模式。这意味着无论选择哪家供应商，短剧内容的转写授权问题都需要业务/法务侧独立解决，不能指望通过供应商选型规避版权风险。
5. **腾讯云"不用于训练"的opt-in承诺目前只在国际站条款中找到明确表述，中国大陆站（cloud.tencent.com）未找到完全对应的书面条款**——建议正式采购前通过腾讯云商务/工单渠道书面确认该条款是否同样适用于大陆账号下的ASR调用。
6. 阶跃云、讯飞两家的数据使用条款相对宽松/明确允许训练使用，若最终选择阶跃云ASR作为成本优化方案，建议同样走商务渠道书面确认"客户数据不用于模型训练"，不能仅依赖当前公开条款推断。
7. 所有价格均为2026-07-23官网现价快照，云厂商定价/套餐/优惠调整频繁，正式采购前需以下单时的实时价格为准。

---

## 附：主要信息来源清单

| 供应商 | 官方文档/定价入口 |
|---|---|
| 阶跃云 StepFun | https://platform.stepfun.com/docs/zh/guides/pricing/details ，https://platform.stepfun.com/docs/zh/api-reference/audio/asr |
| OpenAI Whisper | https://platform.openai.com/docs/guides/speech-to-text ，https://platform.openai.com/docs/guides/your-data |
| 华为云 SIS | https://support.huaweicloud.com/api-sis/sis_03_0092.html ，https://www.huaweicloud.com/pricing/calculator.html#/sis |
| 腾讯云 ASR | https://cloud.tencent.com/document/product/1093/35686 ，https://cloud.tencent.com/document/product/301/94121 |
| 阿里云 ISI/DashScope | https://help.aliyun.com/zh/isi/developer-reference/api-reference-2 ，https://help.aliyun.com/zh/model-studio/paraformer-recorded-speech-recognition-restful-api |
| 讯飞开放平台 | https://www.xfyun.cn/doc/spark/asr_llm/Ifasr_llm.html ，https://www.xfyun.cn/doc/policy/privacy.html |
| DeepSeek | https://api-docs.deepseek.com/quick_start/pricing/ ，https://arxiv.org/abs/2606.19348 |
| 通义千问 Qwen | https://help.aliyun.com/zh/model-studio/model-pricing ，https://www.alibabacloud.com/help/en/model-studio/text-generation-model/ |
| Kimi (Moonshot) | https://platform.kimi.com/docs/pricing/chat-k26 ，https://platform.kimi.com/docs/overview |
| 智谱 GLM | https://docs.bigmodel.cn/cn/guide/models/text/glm-5.2 ，https://docs.bigmodel.cn/cn/guide/develop/openai/introduction |

**调研局限声明**：本报告基于2026-07-23的公开信息、官方文档与学术论文，部分数据（尤其智谱GLM定价、Arena排名快讯）来自二手转述来源，已在正文中逐条标注；营销性质表述与推测结论均已明确标记，不作为最终采购决策的唯一依据，建议关键决策点（准确率实测、合规条款书面确认）在选型定案前独立验证。
