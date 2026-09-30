export namespace main {
  export class SettingsSnapshot {
    openRouterKeyConfigured: boolean;
    sttModel: string;
    analyzerModel: string;
    hardBudgetUsd: number;
    audioRetention: string;
    ocrEnabled: boolean;
    microphoneEnabled: boolean;

    static createFrom(source: any = {}) {
      return new SettingsSnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.openRouterKeyConfigured = source["openRouterKeyConfigured"];
      this.sttModel = source["sttModel"];
      this.analyzerModel = source["analyzerModel"];
      this.hardBudgetUsd = source["hardBudgetUsd"];
      this.audioRetention = source["audioRetention"];
      this.ocrEnabled = source["ocrEnabled"];
      this.microphoneEnabled = source["microphoneEnabled"];
    }
  }
  export class PrivacyState {
    zdrEnabled: boolean;
    audioRetention: string;

    static createFrom(source: any = {}) {
      return new PrivacyState(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.zdrEnabled = source["zdrEnabled"];
      this.audioRetention = source["audioRetention"];
    }
  }
  export class CostSummary {
    currentUsd: number;
    projectedUsd: number;
    hardBudgetUsd: number;
    warning: boolean;
    hardExceeded: boolean;
    currency: string;

    static createFrom(source: any = {}) {
      return new CostSummary(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.currentUsd = source["currentUsd"];
      this.projectedUsd = source["projectedUsd"];
      this.hardBudgetUsd = source["hardBudgetUsd"];
      this.warning = source["warning"];
      this.hardExceeded = source["hardExceeded"];
      this.currency = source["currency"];
    }
  }
  export class VocabularySnapshot {
    id: string;
    simplified: string;
    traditional?: string;
    pinyin: string;
    meaning: string;
    partOfSpeech?: string;
    classifier?: string;
    example: string;
    examplePinyin: string;
    exampleTranslation: string;
    status: string;
    tags: string[];
    lastSeen: string;
    seenCount: number;

    static createFrom(source: any = {}) {
      return new VocabularySnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.simplified = source["simplified"];
      this.traditional = source["traditional"];
      this.pinyin = source["pinyin"];
      this.meaning = source["meaning"];
      this.partOfSpeech = source["partOfSpeech"];
      this.classifier = source["classifier"];
      this.example = source["example"];
      this.examplePinyin = source["examplePinyin"];
      this.exampleTranslation = source["exampleTranslation"];
      this.status = source["status"];
      this.tags = source["tags"];
      this.lastSeen = source["lastSeen"];
      this.seenCount = source["seenCount"];
    }
  }
  export class CandidateSnapshot {
    id: string;
    simplified: string;
    traditional?: string;
    pinyin: string;
    meaning: string;
    partOfSpeech?: string;
    classifier?: string;
    example: string;
    examplePinyin: string;
    exampleTranslation: string;
    provenance: string;
    confidence: number;
    evidence: string;
    timestamp: string;
    bucket: string;
    status: string;
    duplicateOf?: string;
    tags: string[];

    static createFrom(source: any = {}) {
      return new CandidateSnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.simplified = source["simplified"];
      this.traditional = source["traditional"];
      this.pinyin = source["pinyin"];
      this.meaning = source["meaning"];
      this.partOfSpeech = source["partOfSpeech"];
      this.classifier = source["classifier"];
      this.example = source["example"];
      this.examplePinyin = source["examplePinyin"];
      this.exampleTranslation = source["exampleTranslation"];
      this.provenance = source["provenance"];
      this.confidence = source["confidence"];
      this.evidence = source["evidence"];
      this.timestamp = source["timestamp"];
      this.bucket = source["bucket"];
      this.status = source["status"];
      this.duplicateOf = source["duplicateOf"];
      this.tags = source["tags"];
    }
  }
  export class TranscriptLine {
    id: string;
    text: string;
    source: string;
    timestamp: string;
    speaker?: string;
    isMoment?: boolean;

    static createFrom(source: any = {}) {
      return new TranscriptLine(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.text = source["text"];
      this.source = source["source"];
      this.timestamp = source["timestamp"];
      this.speaker = source["speaker"];
      this.isMoment = source["isMoment"];
    }
  }
  export class AudioLevelsSnapshot {
    mic: number;
    remote: number;

    static createFrom(source: any = {}) {
      return new AudioLevelsSnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.mic = source["mic"];
      this.remote = source["remote"];
    }
  }
  export class LessonSnapshot {
    id: string;
    status: string;
    startedAt?: string;
    error?: string;

    static createFrom(source: any = {}) {
      return new LessonSnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.id = source["id"];
      this.status = source["status"];
      this.startedAt = source["startedAt"];
      this.error = source["error"];
    }
  }
  export class ReadinessSnapshot {
    checked: boolean;
    targetAvailable: boolean;
    error?: string;
    checkedAt?: string;

    static createFrom(source: any = {}) {
      return new ReadinessSnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.checked = source["checked"];
      this.targetAvailable = source["targetAvailable"];
      this.error = source["error"];
      this.checkedAt = source["checkedAt"];
    }
  }
  export class AppSnapshot {
    connection: string;
    capturePermission: string;
    readiness: ReadinessSnapshot;
    target: string;
    lesson: LessonSnapshot;
    levels: AudioLevelsSnapshot;
    transcript: TranscriptLine[];
    candidates: CandidateSnapshot[];
    vocabulary: VocabularySnapshot[];
    cost: CostSummary;
    privacy: PrivacyState;
    settings: SettingsSnapshot;
    lastUpdated: string;

    static createFrom(source: any = {}) {
      return new AppSnapshot(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.connection = source["connection"];
      this.capturePermission = source["capturePermission"];
      this.readiness = this.convertValues(
        source["readiness"],
        ReadinessSnapshot,
      );
      this.target = source["target"];
      this.lesson = this.convertValues(source["lesson"], LessonSnapshot);
      this.levels = this.convertValues(source["levels"], AudioLevelsSnapshot);
      this.transcript = this.convertValues(
        source["transcript"],
        TranscriptLine,
      );
      this.candidates = this.convertValues(
        source["candidates"],
        CandidateSnapshot,
      );
      this.vocabulary = this.convertValues(
        source["vocabulary"],
        VocabularySnapshot,
      );
      this.cost = this.convertValues(source["cost"], CostSummary);
      this.privacy = this.convertValues(source["privacy"], PrivacyState);
      this.settings = this.convertValues(source["settings"], SettingsSnapshot);
      this.lastUpdated = source["lastUpdated"];
    }

    convertValues(a: any, classs: any, asMap: boolean = false): any {
      if (!a) {
        return a;
      }
      if (a.slice && a.map) {
        return (a as any[]).map((elem) => this.convertValues(elem, classs));
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

  export class CandidateEditPatch {
    simplified?: string;
    traditional?: string;
    pinyin?: string;
    meaning?: string;
    partOfSpeech?: string;
    classifier?: string;
    example?: string;
    examplePinyin?: string;
    exampleTranslation?: string;
    tags?: string[];

    static createFrom(source: any = {}) {
      return new CandidateEditPatch(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.simplified = source["simplified"];
      this.traditional = source["traditional"];
      this.pinyin = source["pinyin"];
      this.meaning = source["meaning"];
      this.partOfSpeech = source["partOfSpeech"];
      this.classifier = source["classifier"];
      this.example = source["example"];
      this.examplePinyin = source["examplePinyin"];
      this.exampleTranslation = source["exampleTranslation"];
      this.tags = source["tags"];
    }
  }

  export class SettingsPatch {
    openRouterKey?: string;
    sttModel?: string;
    analyzerModel?: string;
    hardBudgetUsd?: number;
    audioRetention?: string;
    ocrEnabled?: boolean;
    microphoneEnabled?: boolean;

    static createFrom(source: any = {}) {
      return new SettingsPatch(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.openRouterKey = source["openRouterKey"];
      this.sttModel = source["sttModel"];
      this.analyzerModel = source["analyzerModel"];
      this.hardBudgetUsd = source["hardBudgetUsd"];
      this.audioRetention = source["audioRetention"];
      this.ocrEnabled = source["ocrEnabled"];
      this.microphoneEnabled = source["microphoneEnabled"];
    }
  }

  export class StartLessonInput {
    target: string;
    consent: boolean;

    static createFrom(source: any = {}) {
      return new StartLessonInput(source);
    }

    constructor(source: any = {}) {
      if ("string" === typeof source) source = JSON.parse(source);
      this.target = source["target"];
      this.consent = source["consent"];
    }
  }
}
