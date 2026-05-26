# Protocol Manual

## Overview

- TCP 用于 `join`、错误、`game_over`
- UDP 用于 `hello`、输入、快照
- 服务端是权威端
- 客户端不要自行判定命中和伤害

## Ports

- TCP: `7000`
- UDP: `7001`

## Join Flow

1. TCP 连接服务端
2. 发送 `JoinReq`
3. 接收 `JoinResp`
4. 保存:
   - `player_id`
   - `session_id`
   - `udp_port`
   - `tick_hz`
5. 打开 UDP
6. 发送 `UdpHello`
7. 持续发送 `InputFrame`
8. 持续接收 `Snapshot`

## Snapshot

`Snapshot` 当前包含两类信息:

- `entities`
  - 表示这一帧结束后世界里的结果态
- `impacts`
  - 表示这一帧内刚刚发生过的命中事件

客户端应该把这两类信息分开使用:

- `entities` 用于更新位置、血量、存在/销毁
- `impacts` 用于播放命中特效、受击闪烁、伤害数字、死亡特效触发

## EntityState

字段:

- `net_id`
- `kind`
- `owner_player_id`
- `pos`
- `vel`
- `hp`
- `radius`

说明:

- `radius` 是服务端逻辑碰撞半径
- 客户端渲染尺寸建议直接基于 `radius`

## ImpactEvent

字段:

- `bullet_net_id`
  - 发生命中的子弹网络 ID
- `bullet_kind`
  - 子弹类型
- `source_player_id`
  - 子弹来源玩家 ID
  - 敌人子弹通常为 `0`
- `target_net_id`
  - 被命中的目标网络 ID
- `target_kind`
  - 被命中的目标类型
- `pos`
  - 命中位置
  - 当前实现使用目标当前位置
- `damage`
  - 本次命中造成的伤害
- `target_destroyed`
  - 这一击后目标是否被打死

## Impact Event Rules

- `impacts` 只表示当前 `snapshot.tick` 这一帧发生过的瞬时事件
- 它不是持久状态，下一帧可能为空
- 客户端应按 `snapshot.tick` 只消费一次
- 不要用“子弹消失”去反推命中，应该优先相信 `impacts`

## Important Notes

- 玩家子弹当前使用分段采样判定
- 敌人子弹当前使用连续碰撞检测
- 玩家子弹在高速时会沿本帧轨迹做多个离散采样点检测，用来减少穿模
- 因此敌人子弹更容易出现“视觉上还没完全贴到目标 sprite，但服务端已经判中”的情况
- 这不是延迟问题，而是服务端判定策略不同
- `ImpactEvent` 就是为了解决这种“结果正确但表现不自然”的客户端同步问题
