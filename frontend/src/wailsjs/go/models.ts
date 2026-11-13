export namespace adjust {
	
	export class AutoStats {
	    black_point: number;
	    white_point: number;
	    gamma: number;
	    temperature: number;
	
	    static createFrom(source: any = {}) {
	        return new AutoStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.black_point = source["black_point"];
	        this.white_point = source["white_point"];
	        this.gamma = source["gamma"];
	        this.temperature = source["temperature"];
	    }
	}
	export class Params {
	    brightness: number;
	    contrast: number;
	    black_point: number;
	    white_point: number;
	    gamma: number;
	    saturation: number;
	    temperature: number;
	    balance_r: number;
	    balance_g: number;
	    balance_b: number;
	    sharpen: number;
	    vignette: number;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.brightness = source["brightness"];
	        this.contrast = source["contrast"];
	        this.black_point = source["black_point"];
	        this.white_point = source["white_point"];
	        this.gamma = source["gamma"];
	        this.saturation = source["saturation"];
	        this.temperature = source["temperature"];
	        this.balance_r = source["balance_r"];
	        this.balance_g = source["balance_g"];
	        this.balance_b = source["balance_b"];
	        this.sharpen = source["sharpen"];
	        this.vignette = source["vignette"];
	    }
	}

}

export namespace filebrowser {
	
	export class FileEntry {
	    name: string;
	    path: string;
	    is_dir: boolean;
	    size: number;
	    mod_time: string;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.is_dir = source["is_dir"];
	        this.size = source["size"];
	        this.mod_time = source["mod_time"];
	    }
	}

}

export namespace finish {
	
	export class Params {
	    ratio?: string;
	    fit?: string;
	    offset?: number;
	    frame?: string;
	    caption?: string;
	    caption_pos?: string;
	    caption_size?: string;
	
	    static createFrom(source: any = {}) {
	        return new Params(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ratio = source["ratio"];
	        this.fit = source["fit"];
	        this.offset = source["offset"];
	        this.frame = source["frame"];
	        this.caption = source["caption"];
	        this.caption_pos = source["caption_pos"];
	        this.caption_size = source["caption_size"];
	    }
	}

}

export namespace generation {
	
	export class AnalyzePrompts {
	    system_prompt: string;
	    single_prompt: string;
	    describe_prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new AnalyzePrompts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.system_prompt = source["system_prompt"];
	        this.single_prompt = source["single_prompt"];
	        this.describe_prompt = source["describe_prompt"];
	    }
	}
	export class ApplyLookParams {
	    image_base64: string;
	    text: string;
	    strength: number;
	    seed?: number;
	
	    static createFrom(source: any = {}) {
	        return new ApplyLookParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.text = source["text"];
	        this.strength = source["strength"];
	        this.seed = source["seed"];
	    }
	}
	export class RecipeStep {
	    preset_id: number;
	    name: string;
	    strength: number;
	
	    static createFrom(source: any = {}) {
	        return new RecipeStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset_id = source["preset_id"];
	        this.name = source["name"];
	        this.strength = source["strength"];
	    }
	}
	export class CompiledRecipe {
	    steps: RecipeStep[];
	    adjust: adjust.Params;
	    source_text?: string;
	
	    static createFrom(source: any = {}) {
	        return new CompiledRecipe(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.steps = this.convertValues(source["steps"], RecipeStep);
	        this.adjust = this.convertValues(source["adjust"], adjust.Params);
	        this.source_text = source["source_text"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CustomStep {
	    preset_id: number;
	    denoising_strength: number;
	    loras?: string;
	    resolution_id?: number;
	
	    static createFrom(source: any = {}) {
	        return new CustomStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset_id = source["preset_id"];
	        this.denoising_strength = source["denoising_strength"];
	        this.loras = source["loras"];
	        this.resolution_id = source["resolution_id"];
	    }
	}
	export class EditImageParams {
	    image_base64: string;
	    reference_image_base64?: string;
	    instruction: string;
	    style_preset_id?: number;
	    style_tags?: boolean;
	    loras?: preset.LoRAEntry[];
	    output_mode?: string;
	    resolution_id?: number;
	    seed?: number;
	    model?: string;
	
	    static createFrom(source: any = {}) {
	        return new EditImageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.reference_image_base64 = source["reference_image_base64"];
	        this.instruction = source["instruction"];
	        this.style_preset_id = source["style_preset_id"];
	        this.style_tags = source["style_tags"];
	        this.loras = this.convertValues(source["loras"], preset.LoRAEntry);
	        this.output_mode = source["output_mode"];
	        this.resolution_id = source["resolution_id"];
	        this.seed = source["seed"];
	        this.model = source["model"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GenerateCompoundImageParams {
	    compound_preset_id: number;
	    extra_prompt: string;
	    extra_negative_prompt: string;
	    resolution_id?: number;
	    hires_profile_id?: number;
	    steps?: CustomStep[];
	    seed?: number;
	    idea?: string;
	    idea_negative?: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateCompoundImageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.compound_preset_id = source["compound_preset_id"];
	        this.extra_prompt = source["extra_prompt"];
	        this.extra_negative_prompt = source["extra_negative_prompt"];
	        this.resolution_id = source["resolution_id"];
	        this.hires_profile_id = source["hires_profile_id"];
	        this.steps = this.convertValues(source["steps"], CustomStep);
	        this.seed = source["seed"];
	        this.idea = source["idea"];
	        this.idea_negative = source["idea_negative"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PresetOverrides {
	    sampler?: string;
	    schedule_type?: string;
	    steps?: number;
	    cfg_scale?: number;
	    clip_skip?: number;
	    model_name?: string;
	    loras?: string;
	
	    static createFrom(source: any = {}) {
	        return new PresetOverrides(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sampler = source["sampler"];
	        this.schedule_type = source["schedule_type"];
	        this.steps = source["steps"];
	        this.cfg_scale = source["cfg_scale"];
	        this.clip_skip = source["clip_skip"];
	        this.model_name = source["model_name"];
	        this.loras = source["loras"];
	    }
	}
	export class GenerateFromImageParams {
	    image_base64: string;
	    mode: string;
	    gen_mode: string;
	    output_mode?: string;
	    preset_id: number;
	    compound_preset_id: number;
	    denoising_strength: number;
	    tags: string;
	    extra_negative_prompt: string;
	    preserve_identity: boolean;
	    resolution_id?: number;
	    hires_profile_id?: number;
	    seed?: number;
	    overrides?: PresetOverrides;
	    mask_base64?: string;
	    keep_color?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GenerateFromImageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.mode = source["mode"];
	        this.gen_mode = source["gen_mode"];
	        this.output_mode = source["output_mode"];
	        this.preset_id = source["preset_id"];
	        this.compound_preset_id = source["compound_preset_id"];
	        this.denoising_strength = source["denoising_strength"];
	        this.tags = source["tags"];
	        this.extra_negative_prompt = source["extra_negative_prompt"];
	        this.preserve_identity = source["preserve_identity"];
	        this.resolution_id = source["resolution_id"];
	        this.hires_profile_id = source["hires_profile_id"];
	        this.seed = source["seed"];
	        this.overrides = this.convertValues(source["overrides"], PresetOverrides);
	        this.mask_base64 = source["mask_base64"];
	        this.keep_color = source["keep_color"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GenerateImageParams {
	    preset_id: number;
	    extra_prompt: string;
	    extra_negative_prompt: string;
	    resolution_id?: number;
	    hires_profile_id?: number;
	    seed?: number;
	    overrides?: PresetOverrides;
	    idea?: string;
	    idea_negative?: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateImageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset_id = source["preset_id"];
	        this.extra_prompt = source["extra_prompt"];
	        this.extra_negative_prompt = source["extra_negative_prompt"];
	        this.resolution_id = source["resolution_id"];
	        this.hires_profile_id = source["hires_profile_id"];
	        this.seed = source["seed"];
	        this.overrides = this.convertValues(source["overrides"], PresetOverrides);
	        this.idea = source["idea"];
	        this.idea_negative = source["idea_negative"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GenerateImageResult {
	    image: string;
	    parameters: number[];
	    info: number[];
	    hires_fix_skipped: boolean;
	    hires_fix_manual: boolean;
	    quality_upscale_skipped?: boolean;
	    quality_redenoise_skipped?: boolean;
	    effective_prompt: string;
	    effective_negative_prompt: string;
	    detected_object?: string;
	    session_item_id?: number;
	
	    static createFrom(source: any = {}) {
	        return new GenerateImageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image = source["image"];
	        this.parameters = source["parameters"];
	        this.info = source["info"];
	        this.hires_fix_skipped = source["hires_fix_skipped"];
	        this.hires_fix_manual = source["hires_fix_manual"];
	        this.quality_upscale_skipped = source["quality_upscale_skipped"];
	        this.quality_redenoise_skipped = source["quality_redenoise_skipped"];
	        this.effective_prompt = source["effective_prompt"];
	        this.effective_negative_prompt = source["effective_negative_prompt"];
	        this.detected_object = source["detected_object"];
	        this.session_item_id = source["session_item_id"];
	    }
	}
	export class GenerateSDPromptParams {
	    preset_id: number;
	    description: string;
	    negative: string;
	    overrides?: PresetOverrides;
	
	    static createFrom(source: any = {}) {
	        return new GenerateSDPromptParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset_id = source["preset_id"];
	        this.description = source["description"];
	        this.negative = source["negative"];
	        this.overrides = this.convertValues(source["overrides"], PresetOverrides);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GenerateSDPromptResult {
	    prompt: string;
	    negative_prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateSDPromptResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt = source["prompt"];
	        this.negative_prompt = source["negative_prompt"];
	    }
	}
	export class ObjectEditParams {
	    image_base64: string;
	    mask_base64: string;
	    op: string;
	    replacement: string;
	    object: string;
	    seed?: number;
	
	    static createFrom(source: any = {}) {
	        return new ObjectEditParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.mask_base64 = source["mask_base64"];
	        this.op = source["op"];
	        this.replacement = source["replacement"];
	        this.object = source["object"];
	        this.seed = source["seed"];
	    }
	}
	
	
	export class RepairParams {
	    image_base64: string;
	    mask_base64: string;
	    strength: number;
	    seed?: number;
	
	    static createFrom(source: any = {}) {
	        return new RepairParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.mask_base64 = source["mask_base64"];
	        this.strength = source["strength"];
	        this.seed = source["seed"];
	    }
	}
	export class UpscaleImageParams {
	    image_base64: string;
	    gen_info: string;
	    preset_id: number;
	
	    static createFrom(source: any = {}) {
	        return new UpscaleImageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.gen_info = source["gen_info"];
	        this.preset_id = source["preset_id"];
	    }
	}

}

export namespace importexport {
	
	export class PresetData {
	    name: string;
	    preset_type: string;
	    type_name: string;
	    prompt: string;
	    negative_prompt: string;
	    sampler: string;
	    schedule_type: string;
	    steps: number;
	    cfg_scale: number;
	    model_name: string;
	    seed?: number;
	    denoising_strength?: number;
	    clip_skip?: number;
	    batch_size?: number;
	    batch_count?: number;
	    vae: string;
	    tags: string;
	    loras: string;
	    source_file?: string;
	
	    static createFrom(source: any = {}) {
	        return new PresetData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.preset_type = source["preset_type"];
	        this.type_name = source["type_name"];
	        this.prompt = source["prompt"];
	        this.negative_prompt = source["negative_prompt"];
	        this.sampler = source["sampler"];
	        this.schedule_type = source["schedule_type"];
	        this.steps = source["steps"];
	        this.cfg_scale = source["cfg_scale"];
	        this.model_name = source["model_name"];
	        this.seed = source["seed"];
	        this.denoising_strength = source["denoising_strength"];
	        this.clip_skip = source["clip_skip"];
	        this.batch_size = source["batch_size"];
	        this.batch_count = source["batch_count"];
	        this.vae = source["vae"];
	        this.tags = source["tags"];
	        this.loras = source["loras"];
	        this.source_file = source["source_file"];
	    }
	}
	export class CompoundStepExportData {
	    step_order: number;
	    denoising_strength: number;
	    loras: string;
	    preset: PresetData;
	
	    static createFrom(source: any = {}) {
	        return new CompoundStepExportData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step_order = source["step_order"];
	        this.denoising_strength = source["denoising_strength"];
	        this.loras = source["loras"];
	        this.preset = this.convertValues(source["preset"], PresetData);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompoundExportData {
	    name: string;
	    description: string;
	    steps: CompoundStepExportData[];
	
	    static createFrom(source: any = {}) {
	        return new CompoundExportData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.steps = this.convertValues(source["steps"], CompoundStepExportData);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompoundImportPreview {
	    pipelines: CompoundExportData[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new CompoundImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pipelines = this.convertValues(source["pipelines"], CompoundExportData);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ExportImageParams {
	    image_base64: string;
	    format: string;
	    width: number;
	    height: number;
	    lock_ratio: boolean;
	    quality: number;
	    interpolation: string;
	    adjust?: adjust.Params;
	    finish?: finish.Params;
	    filename: string;
	    output_dir?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportImageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_base64 = source["image_base64"];
	        this.format = source["format"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.lock_ratio = source["lock_ratio"];
	        this.quality = source["quality"];
	        this.interpolation = source["interpolation"];
	        this.adjust = this.convertValues(source["adjust"], adjust.Params);
	        this.finish = this.convertValues(source["finish"], finish.Params);
	        this.filename = source["filename"];
	        this.output_dir = source["output_dir"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportPreview {
	    presets: PresetData[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.presets = this.convertValues(source["presets"], PresetData);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ValidationWarning {
	    preset_name: string;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ValidationWarning(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset_name = source["preset_name"];
	        this.warnings = source["warnings"];
	    }
	}

}

export namespace kids {
	
	export class CategoryInfo {
	    name: string;
	    label: string;
	    alwaysOn: boolean;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CategoryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	        this.alwaysOn = source["alwaysOn"];
	        this.enabled = source["enabled"];
	    }
	}

}

export namespace llm {
	
	export class LLMModel {
	    id: string;
	    object: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.object = source["object"];
	    }
	}

}

export namespace logger {
	
	export class Entry {
	    id: number;
	    level: string;
	    message: string;
	    timestamp: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.timestamp = source["timestamp"];
	    }
	}

}

export namespace preset {
	
	export class Preset {
	    id: number;
	    name: string;
	    preset_type: string;
	    prompt: string;
	    negative_prompt: string;
	    sampler: string;
	    schedule_type: string;
	    steps: number;
	    cfg_scale: number;
	    model_name: string;
	    seed?: number;
	    denoising_strength?: number;
	    clip_skip?: number;
	    batch_size?: number;
	    batch_count?: number;
	    vae: string;
	    type_id?: number;
	    tags: string;
	    loras: string;
	    is_bundled: boolean;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.preset_type = source["preset_type"];
	        this.prompt = source["prompt"];
	        this.negative_prompt = source["negative_prompt"];
	        this.sampler = source["sampler"];
	        this.schedule_type = source["schedule_type"];
	        this.steps = source["steps"];
	        this.cfg_scale = source["cfg_scale"];
	        this.model_name = source["model_name"];
	        this.seed = source["seed"];
	        this.denoising_strength = source["denoising_strength"];
	        this.clip_skip = source["clip_skip"];
	        this.batch_size = source["batch_size"];
	        this.batch_count = source["batch_count"];
	        this.vae = source["vae"];
	        this.type_id = source["type_id"];
	        this.tags = source["tags"];
	        this.loras = source["loras"];
	        this.is_bundled = source["is_bundled"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class CompoundPresetStep {
	    id: number;
	    compound_preset_id: number;
	    step_order: number;
	    preset_id: number;
	    denoising_strength: number;
	    resolution_id?: number;
	    loras?: string;
	    preset?: Preset;
	
	    static createFrom(source: any = {}) {
	        return new CompoundPresetStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.compound_preset_id = source["compound_preset_id"];
	        this.step_order = source["step_order"];
	        this.preset_id = source["preset_id"];
	        this.denoising_strength = source["denoising_strength"];
	        this.resolution_id = source["resolution_id"];
	        this.loras = source["loras"];
	        this.preset = this.convertValues(source["preset"], Preset);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CompoundPreset {
	    id: number;
	    name: string;
	    description: string;
	    steps: CompoundPresetStep[];
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new CompoundPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.steps = this.convertValues(source["steps"], CompoundPresetStep);
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ExportPreset {
	    id: number;
	    name: string;
	    format: string;
	    width: number;
	    height: number;
	    lock_ratio: boolean;
	    quality: number;
	    interpolation: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new ExportPreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.format = source["format"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.lock_ratio = source["lock_ratio"];
	        this.quality = source["quality"];
	        this.interpolation = source["interpolation"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class StepMeta {
	    step: number;
	    preset?: string;
	    op?: string;
	    model?: string;
	    denoise?: number;
	    loras?: string;
	
	    static createFrom(source: any = {}) {
	        return new StepMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step = source["step"];
	        this.preset = source["preset"];
	        this.op = source["op"];
	        this.model = source["model"];
	        this.denoise = source["denoise"];
	        this.loras = source["loras"];
	    }
	}
	export class GenerationMeta {
	    preset_id?: number;
	    model?: string;
	    label?: string;
	    chain?: StepMeta[];
	    source_item?: number;
	
	    static createFrom(source: any = {}) {
	        return new GenerationMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset_id = source["preset_id"];
	        this.model = source["model"];
	        this.label = source["label"];
	        this.chain = this.convertValues(source["chain"], StepMeta);
	        this.source_item = source["source_item"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HiresProfile {
	    id: number;
	    name: string;
	    upscale: number;
	    denoising_strength: number;
	    upscaler: string;
	    is_builtin: boolean;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new HiresProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.upscale = source["upscale"];
	        this.denoising_strength = source["denoising_strength"];
	        this.upscaler = source["upscaler"];
	        this.is_builtin = source["is_builtin"];
	        this.created_at = source["created_at"];
	    }
	}
	export class HistoryFacet {
	    value: string;
	    name?: string;
	    count: number;
	    pipeline?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryFacet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.name = source["name"];
	        this.count = source["count"];
	        this.pipeline = source["pipeline"];
	    }
	}
	export class HistoryFacets {
	    sessions: HistoryFacet[];
	    models: HistoryFacet[];
	    labels: HistoryFacet[];
	    kinds: HistoryFacet[];
	    shapes: HistoryFacet[];
	    periods: HistoryFacet[];
	
	    static createFrom(source: any = {}) {
	        return new HistoryFacets(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessions = this.convertValues(source["sessions"], HistoryFacet);
	        this.models = this.convertValues(source["models"], HistoryFacet);
	        this.labels = this.convertValues(source["labels"], HistoryFacet);
	        this.kinds = this.convertValues(source["kinds"], HistoryFacet);
	        this.shapes = this.convertValues(source["shapes"], HistoryFacet);
	        this.periods = this.convertValues(source["periods"], HistoryFacet);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HistoryFilter {
	    session_ids: number[];
	    models: string[];
	    labels: string[];
	    kinds: string[];
	    shapes: string[];
	    search: string;
	    date_from: string;
	    date_to: string;
	    period_starts: Record<string, string>;
	    offset: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new HistoryFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_ids = source["session_ids"];
	        this.models = source["models"];
	        this.labels = source["labels"];
	        this.kinds = source["kinds"];
	        this.shapes = source["shapes"];
	        this.search = source["search"];
	        this.date_from = source["date_from"];
	        this.date_to = source["date_to"];
	        this.period_starts = source["period_starts"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	    }
	}
	export class HistoryItem {
	    id: number;
	    session_id: number;
	    file_name: string;
	    thumb_name: string;
	    source: string;
	    prompt: string;
	    negative_prompt: string;
	    sampler: string;
	    steps: number;
	    cfg_scale: number;
	    seed?: number;
	    denoising: number;
	    width: number;
	    height: number;
	    is_preview: boolean;
	    preset_id?: number;
	    is_active: boolean;
	    created_at: string;
	    meta?: GenerationMeta;
	    session_name: string;
	    kind: string;
	    model_name: string;
	    label: string;
	    shape: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.file_name = source["file_name"];
	        this.thumb_name = source["thumb_name"];
	        this.source = source["source"];
	        this.prompt = source["prompt"];
	        this.negative_prompt = source["negative_prompt"];
	        this.sampler = source["sampler"];
	        this.steps = source["steps"];
	        this.cfg_scale = source["cfg_scale"];
	        this.seed = source["seed"];
	        this.denoising = source["denoising"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.is_preview = source["is_preview"];
	        this.preset_id = source["preset_id"];
	        this.is_active = source["is_active"];
	        this.created_at = source["created_at"];
	        this.meta = this.convertValues(source["meta"], GenerationMeta);
	        this.session_name = source["session_name"];
	        this.kind = source["kind"];
	        this.model_name = source["model_name"];
	        this.label = source["label"];
	        this.shape = source["shape"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class HistoryPage {
	    items: HistoryItem[];
	    total: number;
	    facets: HistoryFacets;
	
	    static createFrom(source: any = {}) {
	        return new HistoryPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], HistoryItem);
	        this.total = source["total"];
	        this.facets = this.convertValues(source["facets"], HistoryFacets);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LoRAEntry {
	    name: string;
	    weight: number;
	
	    static createFrom(source: any = {}) {
	        return new LoRAEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.weight = source["weight"];
	    }
	}
	export class PostRecipe {
	    id: number;
	    name: string;
	    steps: string;
	    adjust: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new PostRecipe(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.steps = source["steps"];
	        this.adjust = source["adjust"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	
	export class PresetInstallStatus {
	    id: number;
	    name: string;
	    installed: boolean;
	    missing_sd: string[];
	    missing_lora: string[];
	
	    static createFrom(source: any = {}) {
	        return new PresetInstallStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.installed = source["installed"];
	        this.missing_sd = source["missing_sd"];
	        this.missing_lora = source["missing_lora"];
	    }
	}
	export class PresetType {
	    id: number;
	    name: string;
	    description: string;
	    sort_order: number;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new PresetType(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.sort_order = source["sort_order"];
	        this.created_at = source["created_at"];
	    }
	}
	export class Resolution {
	    id: number;
	    name: string;
	    width: number;
	    height: number;
	    is_builtin: boolean;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Resolution(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.is_builtin = source["is_builtin"];
	        this.created_at = source["created_at"];
	    }
	}
	export class SavedDescription {
	    id: number;
	    text: string;
	    name: string;
	    negative_prompt: string;
	    type: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedDescription(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.name = source["name"];
	        this.negative_prompt = source["negative_prompt"];
	        this.type = source["type"];
	        this.created_at = source["created_at"];
	    }
	}
	export class SavedPrompt {
	    id: number;
	    text: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedPrompt(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.created_at = source["created_at"];
	    }
	}
	export class SessionInfo {
	    id: number;
	    name: string;
	    item_count: number;
	    created_at: string;
	    updated_at: string;
	    is_active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.item_count = source["item_count"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.is_active = source["is_active"];
	    }
	}
	export class SessionItem {
	    id: number;
	    session_id: number;
	    file_name: string;
	    thumb_name: string;
	    source: string;
	    prompt: string;
	    negative_prompt: string;
	    sampler: string;
	    steps: number;
	    cfg_scale: number;
	    seed?: number;
	    denoising: number;
	    width: number;
	    height: number;
	    is_preview: boolean;
	    preset_id?: number;
	    is_active: boolean;
	    created_at: string;
	    meta?: GenerationMeta;
	
	    static createFrom(source: any = {}) {
	        return new SessionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.file_name = source["file_name"];
	        this.thumb_name = source["thumb_name"];
	        this.source = source["source"];
	        this.prompt = source["prompt"];
	        this.negative_prompt = source["negative_prompt"];
	        this.sampler = source["sampler"];
	        this.steps = source["steps"];
	        this.cfg_scale = source["cfg_scale"];
	        this.seed = source["seed"];
	        this.denoising = source["denoising"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.is_preview = source["is_preview"];
	        this.preset_id = source["preset_id"];
	        this.is_active = source["is_active"];
	        this.created_at = source["created_at"];
	        this.meta = this.convertValues(source["meta"], GenerationMeta);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace queue {
	
	export class Job {
	    id: number;
	    type: string;
	    status: string;
	    params: string;
	    progress: number;
	    progress_detail: string;
	    result: string;
	    error: string;
	    source: string;
	    created_at: string;
	    started_at?: string;
	    completed_at?: string;
	    retry_count: number;
	    max_retries: number;
	    next_retry?: string;
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.status = source["status"];
	        this.params = source["params"];
	        this.progress = source["progress"];
	        this.progress_detail = source["progress_detail"];
	        this.result = source["result"];
	        this.error = source["error"];
	        this.source = source["source"];
	        this.created_at = source["created_at"];
	        this.started_at = source["started_at"];
	        this.completed_at = source["completed_at"];
	        this.retry_count = source["retry_count"];
	        this.max_retries = source["max_retries"];
	        this.next_retry = source["next_retry"];
	    }
	}

}

export namespace sd {
	
	export class LoRA {
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new LoRA(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class SDModel {
	    title: string;
	    model_name: string;
	    hash: string;
	    config: string;
	
	    static createFrom(source: any = {}) {
	        return new SDModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.model_name = source["model_name"];
	        this.hash = source["hash"];
	        this.config = source["config"];
	    }
	}
	export class Sampler {
	    name: string;
	    aliases: string[];
	
	    static createFrom(source: any = {}) {
	        return new Sampler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.aliases = source["aliases"];
	    }
	}
	export class Scheduler {
	    name: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new Scheduler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.label = source["label"];
	    }
	}
	export class Upscaler {
	    name: string;
	    model_name: string;
	    scale: number;
	
	    static createFrom(source: any = {}) {
	        return new Upscaler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.model_name = source["model_name"];
	        this.scale = source["scale"];
	    }
	}
	export class VAE {
	    model_name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new VAE(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model_name = source["model_name"];
	        this.path = source["path"];
	    }
	}

}

export namespace serverclient {
	
	export class CatalogLoRA {
	    name: string;
	    base: string;
	    category: string;
	    default_weight: number;
	    category_ru?: string;
	    description: string;
	    description_ru?: string;
	    url?: string;
	    size_mb: number;
	
	    static createFrom(source: any = {}) {
	        return new CatalogLoRA(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.base = source["base"];
	        this.category = source["category"];
	        this.default_weight = source["default_weight"];
	        this.category_ru = source["category_ru"];
	        this.description = source["description"];
	        this.description_ru = source["description_ru"];
	        this.url = source["url"];
	        this.size_mb = source["size_mb"];
	    }
	}
	export class CatalogModule {
	    type: string;
	    filename: string;
	    url: string;
	    size_gb: number;
	
	    static createFrom(source: any = {}) {
	        return new CatalogModule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.filename = source["filename"];
	        this.url = source["url"];
	        this.size_gb = source["size_gb"];
	    }
	}
	export class CatalogSDModel {
	    name: string;
	    base: string;
	    category: string;
	    category_ru?: string;
	    description: string;
	    description_ru?: string;
	    url?: string;
	    size_gb: number;
	    recommended?: boolean;
	    filename?: string;
	    modules?: CatalogModule[];
	    presets?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatalogSDModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.base = source["base"];
	        this.category = source["category"];
	        this.category_ru = source["category_ru"];
	        this.description = source["description"];
	        this.description_ru = source["description_ru"];
	        this.url = source["url"];
	        this.size_gb = source["size_gb"];
	        this.recommended = source["recommended"];
	        this.filename = source["filename"];
	        this.modules = this.convertValues(source["modules"], CatalogModule);
	        this.presets = source["presets"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CatalogModel {
	    name: string;
	    description: string;
	    description_ru?: string;
	    size_gb: number;
	    recommended?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CatalogModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.description_ru = source["description_ru"];
	        this.size_gb = source["size_gb"];
	        this.recommended = source["recommended"];
	    }
	}
	export class Catalog {
	    llm_generate: CatalogModel[];
	    llm_vision: CatalogModel[];
	    sd_models: CatalogSDModel[];
	    lora: CatalogLoRA[];
	
	    static createFrom(source: any = {}) {
	        return new Catalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm_generate = this.convertValues(source["llm_generate"], CatalogModel);
	        this.llm_vision = this.convertValues(source["llm_vision"], CatalogModel);
	        this.sd_models = this.convertValues(source["sd_models"], CatalogSDModel);
	        this.lora = this.convertValues(source["lora"], CatalogLoRA);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class DiscoveredServer {
	    name: string;
	    host: string;
	    port: number;
	    ip_address: string;
	
	    static createFrom(source: any = {}) {
	        return new DiscoveredServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.ip_address = source["ip_address"];
	    }
	}
	export class GPUInfo {
	    name?: string;
	    memory_total_mb?: number;
	    memory_used_mb?: number;
	    memory_free_mb?: number;
	    utilization_percent?: number;
	    available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GPUInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.memory_total_mb = source["memory_total_mb"];
	        this.memory_used_mb = source["memory_used_mb"];
	        this.memory_free_mb = source["memory_free_mb"];
	        this.utilization_percent = source["utilization_percent"];
	        this.available = source["available"];
	    }
	}
	export class HealthResult {
	    healthy: boolean;
	    latency_ms: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.healthy = source["healthy"];
	        this.latency_ms = source["latency_ms"];
	        this.error = source["error"];
	    }
	}
	export class InstallStatus {
	    key: string;
	    installed: boolean;
	    installing: boolean;
	    progress: string;
	    error?: string;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new InstallStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.installed = source["installed"];
	        this.installing = source["installing"];
	        this.progress = source["progress"];
	        this.error = source["error"];
	        this.version = source["version"];
	    }
	}
	export class LLMModelInfo {
	    name: string;
	    size?: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	    }
	}
	export class ModelInfo {
	    name: string;
	    size: number;
	    extension?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.extension = source["extension"];
	    }
	}
	export class ProcessStatus {
	    name: string;
	    status: string;
	    pid?: number;
	    uptime?: string;
	    restarts: number;
	    category?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProcessStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.pid = source["pid"];
	        this.uptime = source["uptime"];
	        this.restarts = source["restarts"];
	        this.category = source["category"];
	    }
	}
	export class ServerModels {
	    sd_checkpoint?: string;
	    llm_running?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ServerModels(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sd_checkpoint = source["sd_checkpoint"];
	        this.llm_running = source["llm_running"];
	    }
	}
	export class ServerStatus {
	    processes: Record<string, ProcessStatus>;
	    health: Record<string, HealthResult>;
	    gpu: GPUInfo;
	    installs: Record<string, InstallStatus>;
	    models: ServerModels;
	
	    static createFrom(source: any = {}) {
	        return new ServerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.processes = this.convertValues(source["processes"], ProcessStatus, true);
	        this.health = this.convertValues(source["health"], HealthResult, true);
	        this.gpu = this.convertValues(source["gpu"], GPUInfo);
	        this.installs = this.convertValues(source["installs"], InstallStatus, true);
	        this.models = this.convertValues(source["models"], ServerModels);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace settings {
	
	export class DownloadItem {
	    type: string;
	    filename: string;
	    url: string;
	    size_gb: number;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.filename = source["filename"];
	        this.url = source["url"];
	        this.size_gb = source["size_gb"];
	        this.model = source["model"];
	    }
	}
	export class DownloadPlan {
	    items: DownloadItem[];
	    total_gb: number;
	
	    static createFrom(source: any = {}) {
	        return new DownloadPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], DownloadItem);
	        this.total_gb = source["total_gb"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LLMChoice {
	    name: string;
	    size?: string;
	    size_gb?: number;
	    installed: boolean;
	    recommended: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LLMChoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.size = source["size"];
	        this.size_gb = source["size_gb"];
	        this.installed = source["installed"];
	        this.recommended = source["recommended"];
	    }
	}
	export class LLMSetup {
	    needed: boolean;
	    generate: LLMChoice[];
	    vision: LLMChoice[];
	    generate_selected: string;
	    vision_selected: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMSetup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.needed = source["needed"];
	        this.generate = this.convertValues(source["generate"], LLMChoice);
	        this.vision = this.convertValues(source["vision"], LLMChoice);
	        this.generate_selected = source["generate_selected"];
	        this.vision_selected = source["vision_selected"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PresetSync {
	    statuses: preset.PresetInstallStatus[];
	    added: number;
	
	    static createFrom(source: any = {}) {
	        return new PresetSync(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.statuses = this.convertValues(source["statuses"], preset.PresetInstallStatus);
	        this.added = source["added"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ServerCheck {
	    state: string;
	    url: string;
	    version?: string;
	    missing?: string[];
	    reason?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.url = source["url"];
	        this.version = source["version"];
	        this.missing = source["missing"];
	        this.reason = source["reason"];
	        this.error = source["error"];
	    }
	}
	export class ServiceInfo {
	    available: boolean;
	    model: string;
	    vision_model?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.model = source["model"];
	        this.vision_model = source["vision_model"];
	    }
	}
	export class ServiceStatus {
	    llm: ServiceInfo;
	    sd: ServiceInfo;
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm = this.convertValues(source["llm"], ServiceInfo);
	        this.sd = this.convertValues(source["sd"], ServiceInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

