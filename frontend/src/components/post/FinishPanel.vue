<script setup>
import { t } from '../../i18n/index.js'

const props = defineProps({
  post: { type: Object, required: true },
})

const { finish, finishRatios, finishActive } = props.post
</script>

<template>
  <div class="form-group">
    <label class="form-label">{{ t('post.finish_ratio') }}</label>
    <select class="form-select" v-model="finish.ratio" data-testid="finish-ratio">
      <option value="">{{ t('post.finish_ratio_none') }}</option>
      <option v-for="r in finishRatios" :key="r" :value="r">{{ t('post.finish_ratio_' + r.replace(':', '_')) }}</option>
    </select>
  </div>
  <template v-if="finish.ratio">
    <div class="form-group">
      <label class="form-label">{{ t('post.finish_fit') }}</label>
      <select class="form-select" v-model="finish.fit" data-testid="finish-fit">
        <option value="crop">{{ t('post.finish_fit_crop') }}</option>
        <option value="blur">{{ t('post.finish_fit_blur') }}</option>
        <option value="white">{{ t('post.finish_fit_white') }}</option>
        <option value="black">{{ t('post.finish_fit_black') }}</option>
      </select>
    </div>
    <div v-if="finish.fit === 'crop'" class="form-group">
      <label class="form-label">{{ t('post.finish_offset') }}</label>
      <input type="range" class="form-range" min="0" max="1" step="0.01" v-model.number="finish.offset" data-testid="finish-offset" />
    </div>
  </template>
  <div class="form-group">
    <label class="form-label">{{ t('post.finish_frame') }}</label>
    <select class="form-select" v-model="finish.frame" data-testid="finish-frame">
      <option value="">{{ t('post.finish_frame_none') }}</option>
      <option value="white">{{ t('post.finish_frame_white') }}</option>
      <option value="black">{{ t('post.finish_frame_black') }}</option>
      <option value="polaroid">{{ t('post.finish_frame_polaroid') }}</option>
    </select>
  </div>
  <div class="form-group">
    <label class="form-label">{{ t('post.finish_caption') }}</label>
    <textarea class="form-control" rows="2" maxlength="200" v-model="finish.caption" data-testid="finish-caption" :placeholder="t('post.finish_caption_ph')"></textarea>
  </div>
  <div v-if="finish.caption.trim()" class="form-row">
    <div v-if="finish.frame !== 'polaroid'" class="form-group form-group-half">
      <label class="form-label">{{ t('post.finish_caption_pos') }}</label>
      <select class="form-select" v-model="finish.caption_pos" data-testid="finish-caption-pos">
        <option value="bottom">{{ t('post.finish_pos_bottom') }}</option>
        <option value="top">{{ t('post.finish_pos_top') }}</option>
        <option value="center">{{ t('post.finish_pos_center') }}</option>
      </select>
    </div>
    <div class="form-group form-group-half">
      <label class="form-label">{{ t('post.finish_caption_size') }}</label>
      <select class="form-select" v-model="finish.caption_size" data-testid="finish-caption-size">
        <option value="s">S</option>
        <option value="m">M</option>
        <option value="l">L</option>
      </select>
    </div>
  </div>
  <div class="form-hint">{{ t('post.finish_hint') }}</div>
  <button v-if="finishActive" class="btn btn-sm btn-secondary" @click="post.resetFinish">{{ t('post.finish_reset') }}</button>
</template>

<style scoped src="./post-shared.css"></style>
