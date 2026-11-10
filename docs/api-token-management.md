# OneImg API Token 多凭据管理与权限规范

> 功能开发分支：`feat/multi-api-token-scopes-expiry-20261110`。未合并、部署前，生产环境仍保持旧接口行为。
> 适用对象：OneImg 管理员、持有 `setting:api` 的用户、ScriptCat / 外部程序开发者。

## 目标与安全基线

- 同一账户可并存多个 Token：每个有名称、备注、独立授权、到期时间、启停和撤销状态。
- 创建新 Token **不会撤销其他 Token**。最多 30 个未过期、未撤销的 Token（暂停也占名额），超额返回 HTTP 409。
- **重复复制不轮换**：新创建的 Token 同时保存 SHA-256 鉴权摘要和由 `CONFIG_SECRET` 派生密钥加密的 AES-256-GCM 副本。已登录且有管理权限的用户可以随时点击“复制 Token”，每次获得的都是同一密钥，Token ID、权限、有效期完全不变。加密副本不会出现在列表、日志或普通查询响应中；仅按需解密返回。
- 支持 5 分钟至 365 天的限期 Token，以及**长期（不过期）**选项。长期必须主动勾选，默认仍为 30 天；长期 Token 可随时暂停、恢复或撤销，最多计入 30 个有效 Token 配额。
- 超管 Token 也**不能**借超管权限访问或删除其他用户的图片，不能操作系统设置或管理其他 Token。
- Token 管理接口只接受已登录的 Cookie 会话，且账户必须具有 `setting:api`；任何 Bearer Token 都不能访问管理接口。
- `start_api=false` 时所有 Bearer Token 停止生效；这不会删除 Token。

## Token 权限矩阵

| scope | 允许的操作 | 说明 |
|---|---|---|
| `image:read` | `GET /api/images`、`GET /api/images/:id` | 只可读取所属账户的图片 |
| `upload:write` | `POST /api/upload`、`POST /api/upload/images`、`POST /api/images/url` | 上传本地图片或导入公网 URL |
| `image:delete` | `DELETE /api/images/:id` | 只可删除所属账户的图片；**单独勾选高风险权限** |
| `folder:read` | `GET /api/folders` | 查看所属账户文件夹 |
| `folder:write` | `GET /api/folders`、`POST /api/folders`、`PUT /api/folders/:id`、`DELETE /api/folders/:id`、`PUT /api/images/folder` | 管理本人文件夹及图片归档 |

补充：`GET /api/uploadConfig`、`GET /api/buckets/list`、`GET /api/tags` 允许 `image:read` **或** `upload:write`；`GET /api/user/status` 对任意有效 Token 开放，用于检查连接。存储桶和文件夹的实际所有权、容量和角色限制继续由各控制器校验。

不支持的权限：`setting:*`、`storage:*`、`user:*`、`*`；API Token 不可用于后台设置、用户/存储管理、创建/编辑全局标签或 R2 直传任务接口。任何不在上述方法及路径白名单中的请求都拒绝。

## 一、凭据管理接口

管理接口统一要求：

```http
Cookie: <已登录 OneImg 后台的同源浏览器会话>
Content-Type: application/json
```

不要将 Bearer Token 当作管理凭据。Token ID 是由随机密钥的摘要衍生的管理标识，**不是 Token 明文**。

### 1.1 获取 Token 列表

`GET /api/tokens?page=1&limit=50`

成功示意：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "tokens": [
      {
        "id": "<opaque 64-character id>",
        "name": "ScriptCat 图片上传",
        "description": "浏览器图片助手",
        "token_hint": "oi_…xYz123",
        "scopes": ["image:read", "upload:write"],
        "expires_at": "2026-12-10T00:00:00Z",
        "last_used_at": null,
        "created_at": "2026-11-10T00:00:00Z",
        "updated_at": "2026-11-10T00:00:00Z",
        "disabled": false,
        "revoked": false,
        "legacy": false,
        "status": "active"
      }
    ],
    "total": 1,
    "page": 1,
    "limit": 50,
    "active_count": 1,
    "max_active": 30,
    "available_scopes": [
      {"code": "image:read", "name": "查看图片", "description": "读取自己的图片、详情、上传配置与标签"}
    ]
  }
}
```

`available_scopes` 实际返回全部可授权权限；样例仅展示一项。Token 状态 `active` / `disabled` / `expired` / `revoked`。

### 1.2 创建 Token

`POST /api/tokens`

```json
{
  "name": "ScriptCat 图片上传",
  "description": "浏览器上传，禁止删除",
  "scopes": ["image:read", "upload:write"],
  "expires_at": "2026-12-10T00:00:00Z"
}
```

响应 **HTTP 201**，JSON 内 `code:200`：

```json
{
  "code": 200,
  "message": "Token 已创建，可随时在后台重新复制",
  "data": {
    "token": "oi_<只显示一次的实际密钥>",
    "api_token": "oi_<相同密钥兼容字段>",
    "credential": {"id":"<opaque id>","name":"ScriptCat 图片上传","scopes":["image:read","upload:write"]}
  }
}
```

创建后及未来登录后台时均可点击“复制 Token”获取**同一个原密钥**，不会导致脚本失效。密钥仍应妥善保管，切勿写入公开代码或日志。

### 1.3 编辑 Token 与长期有效

`PUT /api/tokens/:id`；请求体与创建相同，提交完整 `name`、`description`、`scopes` 与有效期设置。限期 Token 使用 `"never_expires":false` + `"expires_at":"RFC3339 时间"`，有效期最多 365 天；长期 Token 则使用：

```json
{
  "name": "ScriptCat 长期上传",
  "description": "永久有效，上传专用",
  "scopes": ["upload:write", "image:read"],
  "never_expires": true,
  "expires_at": null
}
```

`POST /api/tokens` 新建也接受以上长期选项。长期 Token 的列表响应中 `never_expires:true` 且 `expires_at:null`，后台显示“长期（不过期）”；数据库内部使用带明确标识的远期过期哨兵保障旧版 SQL 校验兼容。

编辑权限与有效期会即时生效，但**不重新生成密钥**。扩大权限尤其是长期 Token 的权限前应谨慎；已经过期、已撤销或 legacy Token 不可编辑。

### 1.4 暂停 / 恢复 Token

`PUT /api/tokens/:id/status`

```json
{"disabled": true}
```

恢复传 `false`。暂停后该 Token 立刻返回 401；恢复后原密钥继续生效。已过期、已撤销的 Token 不能恢复。

### 1.5 永久撤销 Token

`DELETE /api/tokens/:id`

永久撤销不可撤回，相关外部程序立即失去授权；不会影响同一账户其他 Token。

### 1.6 复制原 Token（不会轮换）

后台 Token 列表中点击 **“复制 Token”**，触发：

`POST /api/tokens/:id/reveal`

该接口只接受已登录的 **Cookie 会话** 且需 `setting:api` 权限。它只解密已保存的 AES-GCM 密文，**不轮换、不延长有效期、不改变权限、状态或 ID**；返回 `Cache-Control: no-store` 和 `Pragma: no-cache`，不允许使用 Bearer Token 自助读取。

```json
{
  "code": 200,
  "message": "已读取原 API Token，密钥保持不变",
  "data": {
    "token": "oi_<本次与上次完全相同的密钥>",
    "api_token": "oi_<相同密钥>"
  }
}
```

复制后密钥可暂时显示在界面上供手动复制，点击“已保存，隐藏”即可清除页面展示。请注意：加密副本能够恢复完整密钥，因此后台登录账号及持久化的 `CONFIG_SECRET` 都必须妥善保护。

### 1.7 仅主动操作时才轮换密钥

`POST /api/tokens/:id/rotate` 对应单独的 **“重置密钥”** 按钮，要求二次确认。只有主动确认后才会生成新密钥并立即废弃目标 Token 的旧密钥；权限与有效期保持不变，不影响其他 Token。之后新密钥同样可以重复复制。

**历史 Token 兼容：**此次升级前已创建、只保存摘要的 Token 没有加密副本，无法恢复原密钥。它们仍可正常鉴权；若希望日后重复复制，需要主动使用“重置密钥”一次并同步更新 ScriptCat 配置。旧版 legacy 凭据不可修改或轮换，须创建新的 Token。

### 1.8 兼容旧接口

`POST /api/settings/regenerate` 为历史兼容接口（Cookie 会话），更新后会**新增**一个默认权限、30 天到期的 Token，**不会再全局撤销旧 Token**。新脚本优先使用 `POST /api/tokens`。

旧版 legacy 凭据保留原始有效期与授权，会以“历史 API Token”呈现在列表中，允许撤销，但不能编辑或重置。升级前创建的非 legacy Token 仍正常可用；其原密钥仅在主动轮换一次后才能开启重复复制。

## 二、使用 Token 调用图片 API

新建的 Token 仍按既有格式请求：

```http
Authorization: Bearer oi_your_token_here
```

例如（上传一张图片）：

```bash
curl -X POST "https://img.example.com/api/upload/images" \
  -H "Authorization: Bearer oi_your_token_here" \
  -F "images[]=@sample.png"
```

只读 Token 无法上传；只上传 Token 可以读取 `uploadConfig`、桶列表和标签以选择上传目的地，但不能枚举图片图库；只有显式配置 `image:delete` 才能调用删除。ScriptCat 最小推荐 scope 为 `image:read` + `upload:write`。

## 三、错误与迁移

| 状态码 | 可能原因 |
|---|---|
| 400 | 请求字段无效、scope 未知/重复、到期时间越界 |
| 401 | Token 错误、到期、暂停、撤销，或 API 总开关关闭 |
| 403 | 登录会话缺少 `setting:api`、API scope 不足或跨账户访问 |
| 404 | Token ID 不存在，或不是当前管理用户的 Token |
| 409 | 单账户有效 Token 上限 30 个；legacy Token 不能编辑 |
| 500 | 数据库或服务异常 |

数据库使用 GORM `AutoMigrate`，在原 `api_credentials` 表追加 `encrypted_token` 和 `never_expires` 字段；旧 Token 摘要、权限及有效期不被覆盖。新 Token 密文绑定到自身 SHA-256 ID，密钥来自持续保存的 `CONFIG_SECRET`；丢失或更换此密钥将导致无法再复制历史 Token 的原密钥，但不会自动轮换或撤销它们。升级前请备份数据库与 `data/.config_secret`，升级后确认已有 ScriptCat Token 仍能上传。

**脚本跨站请求提示：**多 Token 改造本身不改变 OneImg 的 `SameOriginWrites` / `Sec-Fetch-Site` 策略。ScriptCat 的 `GM_xmlhttpRequest` 仍须在真实浏览器中验证；浏览器扩展的跨站请求可能被 OneImg 的 Origin 检查拦截。不要因此全局关闭跨站请求保护。

源码位置：
- `backend/models/APICredential.go`
- `backend/utils/authsecurity/tokens.go`
- `backend/controllers/auth/APITokens.go`
- `backend/middlewares/AuthMiddleware.go`
- `backend/routes/routes.go`
- `frontend/src/components/settings/APITokenManager.vue`

相关文档：[ScriptCat 图片上传 API](oneimg-scriptcat-api.md)。
