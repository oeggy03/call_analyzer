# GCP T4 置换会议词汇

简体中文。词条和例句都按这个项目来写：2027 年 GCP 上 SG1 与美东 USEAST2A 的 T4 到期、伯纳德、ML engine、挪威 A10、泰国 L20 / L20N、以及还在收尾的 L4（智创、IES 中台、B40）。

来源：2026-10-09 会议。例句是按会议结论改写的，用来学这些词在这个项目里怎么用。

同音词：**下限** xiàxiàn 是还必须保住的保有量；**下线** xiàxiàn 是把一批卡从 GCP 上撤下来。

| 词 | 拼音 | English | 分组 | 在这个项目里 | 例句 |
| --- | --- | --- | --- | --- | --- |
| 置换 | zhìhuàn | swap / reallocate | 动作 | The name of this whole thread: cover an expiring T4 with a different card. | 科科要在 2027 年一季度前，把 SG1 和美东 USEAST2A 上到期的 T4 置换掉。 |
| 到期 | dàoqī | to expire (a contract) | 动作 | The T4 CUD ends in March and June 2027. 到期 is the contract date, not the day the service is turned off. | 美东这 4200 张 T4 的合同三月初到期，SG1 的 5320 张大多也在一季度到期。 |
| 拉闸 | lāzhá | hard cutoff | 动作 | The backstop: on this date the platform takes the cards whether the business is ready or not. | 2027 年这批 T4，科科准备一月底拉闸，好在春节前把机器从 GCP 上拿下来。 |
| 切流 | qiēliú | shift live traffic | 动作 | The Singapore plan is not a card-for-card swap. Traffic moves to Thailand, and the T4s come back with it. | SG1 的 T4 不在原地换卡，而是跟泰国切流绑在一起，流量切走再退卡。 |
| 适配 | shìpèi | adapt a service to a new GPU | 动作 | The waiting period before 拉闸. A new card is useless until the model and the service can run on it. | 视频架构换上 L20 之前要先适配；伯纳德如果只拿到大卡，适配会更难。 |
| 腾挪 | téngnuó | shuffle capacity internally | 动作 | The business must move services so that the cards which free up are these T4s, not some other pool. | 切流之前，搜广推和伯纳德得先把服务腾挪好，放出来的必须是 SG1 上这批 T4。 |
| 算力 | suànlì | computing power | 动作 | The unit they actually swap. Card model stays open; the budget is filed as A30-class and A100-class compute. | 一张 T4 按 0.6 张 A10 的算力来置换，具体卡型可以后定。 |
| 口径 | kǒujìng | official framing | 动作 | The sentence the notice is allowed to say. Singapore's 口径 is the Thailand cutover; Europe's 口径 is Norway now, large cards in 2027. | 发给金主的口径是：挪威先用 A10 顶上 T4，2027 年再用大卡把预算补回来。 |
| 小卡 | xiǎokǎ | small / older GPU | 卡和供应 | T4, and in this room also A10 and L20. Europe and the US have almost no 小卡 supply next year. | T4、A10、L20 都算小卡，明年欧盟和美东几乎没有小卡可以交付。 |
| 大卡 | dàkǎ | high-end GPU | 卡和供应 | B300, GB300, VR200, and the A100 budget bucket. Fine for training; wasteful for Bernard inference. | GB300、B300 这些大卡可以给 ML engine 做训练，伯纳德拿去推理会浪费。 |
| 卡型 | kǎxíng | GPU model / SKU | 卡和供应 | What they deliberately refuse to lock. The budget form takes two compute lines, not a named 卡型. | T4 的预算表里不填具体卡型，只报 A30 和 A100 两档算力。 |
| 实卡 | shíkǎ | physical cards in hand | 卡和供应 | Norway A10s exist now. Next year's B300 does not. The deal spends 实卡 first. | 挪威现在大约有一千张空闲 A10 实卡，可以先拿来顶美东伯纳德的 T4。 |
| 交付 | jiāofù | hand supply over | 卡和供应 | The moment a promised card becomes usable. B40 交付 is 12 Oct and 20 Oct; the US large-card 交付 date does not exist yet. | B40 十月十二号开始交付，美东那批大卡的交付时间还没有。 |
| 囤 | tún | stockpile | 卡和供应 | 汝杰 will hold Norway A10s aside so they are still there when 科科 trades them for the T4s. | 汝杰准备把挪威的 A10 囤下来，专门用来置换美东伯纳德的 T4。 |
| 退卡 | tuìkǎ | return cards | 卡和供应 | The condition attached to the Thailand cut: traffic moves, this batch comes back. | 泰国切流之后，金主要把 SG1 这批 T4 退回来，不能拿别的池子充数。 |
| 续 | xù | renew | 卡和供应 | The alternative to 退卡. VA A10 was already decided as 续, so this meeting ignores it. | VA 上的云 T4 已经下完了；剩下的 A10 之前决定续，不进这次 T4 置换。 |
| 下卡 | xiàkǎ | take cards off a tenant | 卡和供应 | Who loses the card. 科科 expects the EU pull to start on the ML engine side. | 美东这批大概率先从 ML engine 的小卡训练下卡，再补给伯纳德。 |
| 下限 | xiàxiàn | committed floor | 预算 | The minimum the business must still be covered for. Same sound as 下线 (take offline). Here it means the floor, not the shutdown. | 挪威 A10 先守住业务 T4 的下限，缺的算力 2027 年用大卡补。 |
| 下线 | xiàxiàn | take offline | 动作 | Homophone of 下限. GCB 下线 is why the US clear-out is 2,752 rather than the full 4,200. | 前面 GCB 已经下线了一批，美东 4200 张 T4 里现在只要清 2752 张。 |
| 抵扣 | dǐkòu | offset | 预算 | Return cards here only if you are paid back there. 汝杰 rejected a 2026 抵扣 because he cannot deliver GB300 this year. | 推荐想退一千张卡来抵扣，换汝杰今年在 VA 交 GB300，他没答应。 |
| 期货 | qīhuò | futures | 预算 | Take Norway A10 now; the large-card repayment is a 2027 promise. | 业务先拿挪威 A10 顶上 T4，等于接受期货，大卡 2027 年再还。 |
| 打平 | dǎpíng | balance the books | 预算 | The futures deal works only if 科科 books the repayment, so nobody is left short. | 科科把挪威这笔单独写进 2027 年新增供应，账才能和大卡打平。 |
| 提笔 | tíbǐ | raise a budget line | 预算 | 科科's homework: a separate 2027 new-supply line that covers the Norway payback. | 挪威 A10 换 T4 的这笔钱要单独提笔，不能混进别的 GPU 预算。 |
| 预算 | yùsuàn | budget | 预算 | Filed as two compute buckets, A30 and A100, for every card expiring next year, as one package. | 明年到期的 SG1 和美东 T4 打成一个包报预算，只填 A30 和 A100 的算力。 |
| 财税 | cáishuì | finance and tax | 预算 | Why the payer, not the platform, has to return this batch: tax treatment plus the GCP contract. | 因为财税，再加上 GCP 合同到期，SG1 这批 T4 必须由金主自己退。 |
| 金主 | jīnzhǔ | paying business / KA | 团队 | The team that holds the budget. They are who the notice is written to, and who must return the cards. | 科科发通知的对象是金主，泰国切流的时间和 T4 拉闸的时间都要写给他们。 |
| 大头 | dàtóu | the bulk | 团队 | Who actually matters. US: ML engine then Bernard. Singapore: Bernard then video architecture. | SG1 这 5320 张 T4 里，大头在伯纳德，视频架构大约一千张。 |
| 中台 | zhōngtái | shared mid-platform | 团队 | IES 中台's lot is about 381 cards. Part of the 12 Oct B40 batch is for them. | IES 中台这大约 381 张 L4，要分到十月十二号交付的 B40。 |
| 搜广推 | sōu guǎng tuī | search, ads, recommendation | 团队 | The traffic owners. Their Norway A10s went idle after the cut to 2B. The transcript misheard this as 缩管推. | 流量切到 2B 之后，搜广推在挪威用不上的 A10，正好拿来置换 T4。 |
| 视频架构 | shìpín jiàgòu | Video Architecture | 团队 | The easy Singapore holder, about 1,000 cards. Almost any GPU works if they get adaptation time. | 视频架构在 SG1 大约一千张 T4，换成泰国的 L20，留出适配时间就行。 |
| 智创 | zhìchuàng | Intelligent Creation | 团队 | Wanted to keep 290 shared SG1 L4s. This meeting removed that option. | 智创原先想在 SG1 留 290 张 L4，这次不行，全部适配到 B40。 |
| 利用率 | lìyònglǜ | utilization | 动作 | Why a voluntary return will not happen. Teams are pushing this number up, so idle cards get absorbed. | 业务在卷利用率，所以不会主动把挪威的空闲 A10 退出来换 T4。 |
| 建站 | jiànzhàn | stand up a site | 卡和供应 | Thailand. November is when the new site can start taking traffic, which is what makes the Q1 cut real. | 泰国十一月开始建站接流量，三月前要把 SG1 的 T4 随着切流退回来。 |
| 开机 | kāijī | power machines on | 卡和供应 | 椿枚 asked; 汝杰 said boot the B40s now. Cards that are not 开机 are not delivered. | 椿枚问十月十二号的 B40 要不要开机，汝杰说赶紧开，先交付给广告。 |
| 提单 | tídān | file a ticket | 动作 | 少峰's job for the B40 batch. In this programme a return or a delivery is not real until the ticket exists. | 少峰要赶紧给这批 B40 提单，IES 中台的份额等卡到了再分。 |
| 标的 | biāodì | the lot in question | 团队 | 霍浩's 381 is one 标的: a named package of cards, not a vague team total. | IES 中台这 381 张是一个单独的标的，跟智创那批 SG1 L4 分开看。 |
| 机房 | jīfáng | data center / site | 卡和供应 | 霍浩 wants the swap written per 机房. Norway, 2A, 2B, Thailand, and SG1 are different answers. | 置换策略要按机房写：挪威给 A10，泰国给 L20，美东大卡的时间另说。 |
| 黑五 | Hēiwǔ | Black Friday | 动作 | The peak they are waiting out. After it, Q1 traffic growth is manageable, so the Thailand cut can land. | 黑五之后流量增长有限，所以一季度可以一边泰国切流，一边把 SG1 的 T4 换掉。 |
| 倒腾 | dǎoteng | reshuffle by hand | 动作 | Informal 腾挪. Training can 倒腾 onto a big card; Bernard inference cannot. | ML engine 的小卡训练还能自己倒腾到大卡上，伯纳德的在线推理倒腾不过来。 |
| 排期 | páiqī | schedule | 动作 | 智创's adaptation 排期 is the risk: overseas headcount versus a 4 Nov 拉闸. | 智创把 SG1 的 L4 适配到 B40，排期可能赶不上十一月四号拉闸。 |

