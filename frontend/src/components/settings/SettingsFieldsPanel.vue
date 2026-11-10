<template>
<!-- 通用表单面板：只服务「表单 + 开关」两栏的那几个页签。
     security 的 Referer 白名单已并入 SettingsSecurityPanel，seo/system 各自单列，
     这三种情况下面板不渲染，避免留下一个只有标题的空卡片。 -->
<div v-if="hasFormPanel" class="w-full">
                    <div class="surface p-3.5 sm:p-4 md:p-6">
                        <h2 class="panel-title mb-4 md:mb-5">
                            <span class="panel-icon"><i class="ri-list-settings-line"></i></span>
                            {{ activeSettingsTabLabel }}
                        </h2>

                        <div class="space-y-4 md:space-y-5">
                            <!-- ========== 上传与存储 ==========
                                 两列排布：单列时每个输入框 641px，一行一个要滚很久；
                                 路径/文件名这组都是短值，两列更紧凑。 -->
                            <div v-show="activeSettingsTab === 'storage'" class="grid gap-4 md:grid-cols-2 md:gap-5">
                                <div class="setting-group">
                                    <label class="field-label" for="default_storage">系统默认存储</label>
                                    <select id="default_storage" v-model="systemSettings.default_storage" class="input-modern" @change="handleSelectChange('default_storage', systemSettings.default_storage)">
                                        <option v-for="bucket in presetBuckets" :key="bucket.id" :value="bucket.id">{{ bucket.name }} ({{ bucket.type }})</option>
                                    </select>
                                </div>
                                <div class="setting-group">
                                    <label class="field-label" for="public_image_domain">图片直链域名</label>
                                    <input
                                        id="public_image_domain"
                                        v-model="systemSettings.public_image_domain"
                                        type="text"
                                        class="input-modern"
                                        :class="{ 'cursor-not-allowed opacity-60': publicImageDomainInputDisabled }"
                                        :disabled="publicImageDomainInputDisabled"
                                        placeholder="例如 https://img.example.com"
                                        @blur="handleFieldBlur('public_image_domain', systemSettings.public_image_domain)"
                                    />
                                    <div
                                        class="field-hint"
                                        :class="{ 'text-amber-700 dark:text-amber-300': publicImageDomainUnavailable || hasPublicImageDomain }"
                                    >
                                        {{ publicImageDomainHint }}
                                    </div>
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="default_path">默认存储路径</label>
                                    <input id="default_path" v-model="systemSettings.default_path" type="text" class="input-modern" placeholder="/uploads/{year}/{moon}" @blur="handleFieldBlur('default_path', systemSettings.default_path)" />
                                    <div class="field-hint">魔法变量：{year} {month} {day} {hour} {minute} {random} {uuid}</div>
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="file_name">上传文件名称</label>
                                    <input id="file_name" v-model="systemSettings.file_name" type="text" class="input-modern" placeholder="{random}" @blur="handleFieldBlur('file_name', systemSettings.file_name)" />
                                    <div class="field-hint">魔法变量：{random} {filename} {year} {month} {day} {hour} {minute} {second}</div>
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="allowed_types">允许上传的图片类型</label>
                                    <input id="allowed_types" v-model="systemSettings.allowed_types" type="text" class="input-modern" placeholder="image/jpeg,image/png…（SVG 不允许）" @blur="handleFieldBlur('allowed_types', systemSettings.allowed_types)" />
                                    <div class="field-hint">多个 MIME 类型以英文逗号分隔</div>
                                </div>

                                <!-- 三个数值型字段：三列，窄屏堆叠 -->
                                <div class="grid gap-4 sm:grid-cols-3 md:gap-3">
                                    <div class="setting-group">
                                        <label class="field-label" for="max_file_size">最大上传大小</label>
                                        <input id="max_file_size" v-model="systemSettings.max_file_size" type="number" class="input-modern" placeholder="按字节计算" @blur="handleFieldBlur('max_file_size', systemSettings.max_file_size)" />
                                        <div class="field-hint">字节，默认 10485760</div>
                                    </div>
                                    <div v-for="field in [{ key: 'max_upload_files', label: '每批文件数' }, { key: 'tag_max_length', label: '标签字符数' }]" v-show="systemSettings[field.key] !== undefined" :key="field.key" class="setting-group">
                                        <label class="field-label" :for="field.key">{{ field.label }}</label>
                                        <input :id="field.key" v-model.number="systemSettings[field.key]" type="number" min="1" max="50" step="1" class="input-modern" @blur="handleFieldBlur(field.key, systemSettings[field.key])" />
                                    </div>
                                </div>

                                <!-- 账号默认上传配额。
                                     单独一行并说明优先级，因为「默认」和「单用户覆盖」的关系
                                     不写清楚的话，管理员会误以为这里改了对已设配额的账号也生效。 -->
                                <div class="setting-group">
                                    <label class="field-label" for="default_user_quota_gb">账号默认上传配额</label>
                                    <div class="flex items-center gap-2">
                                        <input
                                            id="default_user_quota_gb"
                                            :value="defaultQuotaGb"
                                            type="number"
                                            min="0"
                                            step="0.1"
                                            class="input-modern w-32 shrink-0"
                                            @blur="handleDefaultQuotaBlur"
                                        />
                                        <span class="text-sm text-slate-500 dark:text-slate-400">GB</span>
                                        <span class="text-xs text-slate-400 dark:text-slate-500">0 表示不限</span>
                                    </div>
                                    <div class="field-hint">
                                        未单独设置配额的账号按此额度计算；已单独设置过的账号以各自配额为准，不受这里影响。管理员不受配额限制。
                                    </div>
                                </div>
                            </div>

                            <!-- ========== 通知 ========== -->
                            <div v-show="activeSettingsTab === 'notifications'" class="grid gap-4 md:grid-cols-2 md:gap-5">
                                <div class="setting-group">
                                    <label class="field-label" for="tg_bot_token">TG Bot Token</label>
                                    <input id="tg_bot_token" v-model="systemSettings.tg_bot_token" type="text" class="input-modern" :placeholder="systemSettings.tg_bot_token_configured ? '已配置，留空表示不修改' : '未配置，请输入 Bot Token'" @blur="handleFieldBlur('tg_bot_token', systemSettings.tg_bot_token)" />
                                    <div class="field-hint">{{ systemSettings.tg_bot_token_configured ? '已配置，留空表示不修改' : '发送 Telegram 通知时必填' }}</div>
                                </div>
                                <div class="setting-group">
                                    <label class="field-label" for="tg_receivers">TG 通知接收者</label>
                                    <input id="tg_receivers" v-model="systemSettings.tg_receivers" type="text" class="input-modern" placeholder="接收通知的 TG 用户 ID" @blur="handleFieldBlur('tg_receivers', systemSettings.tg_receivers)" />
                                    <div class="field-hint">多个接收者以英文逗号分隔</div>
                                </div>
                                <div class="setting-group md:col-span-2">
                                    <label class="field-label" for="tg_notice_text">TG 通知文本</label>
                                    <input id="tg_notice_text" v-model="systemSettings.tg_notice_text" type="text" class="input-modern" placeholder="自定义 TG 通知文本" @blur="handleFieldBlur('tg_notice_text', systemSettings.tg_notice_text)" />
                                    <div class="field-hint">默认模板：{username} {date} 上传了图片 {filename}，存储容器 [{StorageType}]</div>
                                </div>
                            </div>

                            <!-- ========== 图片处理 ==========
                                 水印文本占整行（可能较长），其余四项 2×2。 -->
                            <div v-show="activeSettingsTab === 'image'" class="grid gap-4 md:grid-cols-2 md:gap-5">
                                <div class="setting-group md:col-span-2">
                                    <label class="field-label" for="watermark_text">图片水印文本</label>
                                    <input id="watermark_text" v-model="systemSettings.watermark_text" type="text" class="input-modern" :class="{ 'cursor-not-allowed opacity-60': hasPublicImageDomain }" :disabled="hasPublicImageDomain" placeholder="图片水印文本" @blur="handleFieldBlur('watermark_text', systemSettings.watermark_text)" />
                                    <div v-if="hasPublicImageDomain" class="field-hint text-amber-700 dark:text-amber-300">已配置图片直链域名，水印不会生效，请先清空直链域名再修改。</div>
                                    <div v-else class="field-hint">仅作用于新上传的图片，历史图片不会补加。</div>
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="watermark_size">图片水印大小</label>
                                    <input id="watermark_size" v-model="systemSettings.watermark_size" type="text" class="input-modern" :class="{ 'cursor-not-allowed opacity-60': hasPublicImageDomain }" :disabled="hasPublicImageDomain" placeholder="例如 10" @blur="handleFieldBlur('watermark_size', systemSettings.watermark_size)" />
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="watermark_color">字体颜色</label>
                                    <input id="watermark_color" v-model="systemSettings.watermark_color" type="text" class="input-modern" :class="{ 'cursor-not-allowed opacity-60': hasPublicImageDomain }" :disabled="hasPublicImageDomain" placeholder="#000000" @blur="handleFieldBlur('watermark_color', systemSettings.watermark_color)" />
                                    <div class="field-hint">十六进制色值，默认 #000000</div>
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="watermark_opac">水印透明度</label>
                                    <input id="watermark_opac" v-model="systemSettings.watermark_opac" type="text" class="input-modern" :class="{ 'cursor-not-allowed opacity-60': hasPublicImageDomain }" :disabled="hasPublicImageDomain" placeholder="0.5" @blur="handleFieldBlur('watermark_opac', systemSettings.watermark_opac)" />
                                    <div class="field-hint">0–1 之间，默认 0.5</div>
                                </div>

                                <div class="setting-group">
                                    <label class="field-label" for="watermark_pos">水印位置</label>
                                    <select id="watermark_pos" v-model="systemSettings.watermark_pos" class="input-modern" :class="{ 'cursor-not-allowed opacity-60': hasPublicImageDomain }" :disabled="hasPublicImageDomain" @change="handleSelectChange('watermark_pos', systemSettings.watermark_pos)">
                                        <option value="" disabled>请选择位置</option>
                                        <option value="top-left">左上角</option>
                                        <option value="top-right">右上角</option>
                                        <option value="bottom-left">左下角</option>
                                        <option value="bottom-right">右下角</option>
                                        <option value="center">居中</option>
                                    </select>
                                    <div class="field-hint">系统默认右下角</div>
                                </div>
                            </div>

                            <!-- ========== 安全与登录：表单部分已并入 SettingsSecurityPanel ========== -->

                            <!-- ========== API ========== -->
                            <div v-show="activeSettingsTab === 'api'" class="setting-group">
                                <label class="field-label" for="random_image_limit">随机图最大返回数量</label>
                                <input v-if="systemSettings.random_image_limit !== undefined" id="random_image_limit" v-model.number="systemSettings.random_image_limit" type="number" min="1" max="100" step="1" class="input-modern" @blur="handleFieldBlur('random_image_limit', systemSettings.random_image_limit)" />
                                <div class="field-hint">仅 <code>model=json</code> 时生效，超出上限按上限返回。</div>
                            </div>

                            <!-- 多 Token 的创建、编辑、状态和密钥显示使用独立 APITokenManager 面板。 -->
                            <div v-show="activeSettingsTab === 'api'" class="setting-group">
                                <label class="field-label" for="random-graph-config">随机图范围</label>
                                <button id="random-graph-config" type="button" class="soft-button w-full sm:w-auto" @click="generateRandomGraph">配置随机图</button>
                                <div class="field-hint">
                                    随机图接口：<a class="font-medium text-primary underline-offset-2 hover:underline" href="/api/images/random" target="_blank" rel="noopener">/api/images/random</a>
                                </div>
                                <ul class="mt-2 space-y-1 rounded-[var(--radius-sm)] border border-slate-200 bg-slate-50 px-3 py-2.5 text-xs leading-5 text-slate-500 dark:border-white/10 dark:bg-slate-950 dark:text-slate-400">
                                    <li><code class="text-slate-700 dark:text-slate-300">tag</code> · text 标签分类</li>
                                    <li><code class="text-slate-700 dark:text-slate-300">model</code> · json / image 返回类型（数据或图片流）</li>
                                    <li><code class="text-slate-700 dark:text-slate-300">limit</code> · int 返回数量，默认 1，最大 {{ systemSettings.random_image_limit || 20 }}，仅 model=json 时生效</li>
                                </ul>
                            </div>
                        </div>
                    </div>
                </div>
</template>
<script setup>
import { computed, inject } from 'vue';
import { settingsContext } from '@/utils/settingsAccess.js';
import { bytesToGb, gbToBytes } from '@/utils/byteSize.js';
const { systemSettings, activeSettingsTab, activeSettingsTabLabel, presetBuckets, hasPublicImageDomain, publicImageDomainUnavailable, publicImageDomainInputDisabled, publicImageDomainHint, handleFieldBlur, handleSelectChange, generateRandomGraph } = inject(settingsContext);

// 这些页签有本面板要渲染的表单字段
const FORM_TABS = ['storage', 'image', 'notifications', 'api'];
const hasFormPanel = computed(() => FORM_TABS.includes(activeSettingsTab.value));

// 配额在数据库里存字节，界面上用 GB 输入。
// 直接绑 v-model 到字节数会让用户输入「10」却得到 10 字节 —— 这是最容易
// 出错的地方，所以用一个只读展示值 + blur 时换算回写的模式。
const defaultQuotaGb = computed(() => bytesToGb(systemSettings.value.default_user_quota, 2));

function handleDefaultQuotaBlur(event) {
  const raw = String(event.target.value ?? '').trim();
  const gb = Number(raw);
  if (raw === '' || !Number.isFinite(gb) || gb < 0) {
    // 非法输入不提交，把输入框还原成当前真实值，避免界面上显示一个
    // 从未生效过的数字。
    event.target.value = defaultQuotaGb.value;
    return;
  }
  // 先更新本地值，让输入框立刻反映换算后的结果（例如 10.005 → 10.0）。
  systemSettings.value.default_user_quota = gbToBytes(gb);
  handleFieldBlur('default_user_quota', gbToBytes(gb));
  event.target.value = bytesToGb(gbToBytes(gb), 2);
}
</script>
