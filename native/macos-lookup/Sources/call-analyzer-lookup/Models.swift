import Foundation

struct SelectionBounds: Codable, Sendable {
    let x: Double
    let y: Double
    let width: Double
    let height: Double
}

struct GlossLine: Codable, Equatable, Sendable {
    let surface: String
    let simplified: String
    let traditional: String?
    let pinyin: String
    let meaning: String
    let partOfSpeech: String?
    let classifier: String?
    let example: String?
    let examplePinyin: String?
    let exampleTranslation: String?
    let tags: [String]?
    let vocabularyId: String?
    let action: String
    let destination: String

    enum CodingKeys: String, CodingKey {
        case surface
        case simplified
        case traditional
        case pinyin
        case meaning
        case partOfSpeech
        case classifier
        case example
        case examplePinyin
        case exampleTranslation
        case tags
        case vocabularyId
        case action
        case destination
    }

    init(
        surface: String,
        simplified: String,
        traditional: String?,
        pinyin: String,
        meaning: String,
        partOfSpeech: String? = nil,
        classifier: String? = nil,
        example: String? = nil,
        examplePinyin: String? = nil,
        exampleTranslation: String? = nil,
        tags: [String]? = nil,
        vocabularyId: String?,
        action: String,
        destination: String
    ) {
        self.surface = surface
        self.simplified = simplified
        self.traditional = traditional
        self.pinyin = pinyin
        self.meaning = meaning
        self.partOfSpeech = partOfSpeech
        self.classifier = classifier
        self.example = example
        self.examplePinyin = examplePinyin
        self.exampleTranslation = exampleTranslation
        self.tags = tags
        self.vocabularyId = vocabularyId
        self.action = action
        self.destination = destination
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        surface = try container.decode(String.self, forKey: .surface)
        simplified = try container.decode(String.self, forKey: .simplified)
        traditional = try container.decodeIfPresent(String.self, forKey: .traditional)
        pinyin = try container.decode(String.self, forKey: .pinyin)
        meaning = try container.decode(String.self, forKey: .meaning)
        partOfSpeech = try container.decodeIfPresent(String.self, forKey: .partOfSpeech)
        classifier = try container.decodeIfPresent(String.self, forKey: .classifier)
        example = try container.decodeIfPresent(String.self, forKey: .example)
        examplePinyin = try container.decodeIfPresent(String.self, forKey: .examplePinyin)
        exampleTranslation = try container.decodeIfPresent(String.self, forKey: .exampleTranslation)
        tags = try container.decodeIfPresent([String].self, forKey: .tags)
        vocabularyId = try container.decodeIfPresent(String.self, forKey: .vocabularyId)
        action = try container.decode(String.self, forKey: .action)
        destination = try container.decode(String.self, forKey: .destination)
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(surface, forKey: .surface)
        try container.encode(simplified, forKey: .simplified)
        try container.encodeIfPresent(traditional, forKey: .traditional)
        try container.encode(pinyin, forKey: .pinyin)
        try container.encode(meaning, forKey: .meaning)
        try container.encodeIfPresent(partOfSpeech, forKey: .partOfSpeech)
        try container.encodeIfPresent(classifier, forKey: .classifier)
        try container.encodeIfPresent(example, forKey: .example)
        try container.encodeIfPresent(examplePinyin, forKey: .examplePinyin)
        try container.encodeIfPresent(exampleTranslation, forKey: .exampleTranslation)
        try container.encodeIfPresent(tags, forKey: .tags)
        try container.encodeIfPresent(vocabularyId, forKey: .vocabularyId)
        try container.encode(action, forKey: .action)
        try container.encode(destination, forKey: .destination)
    }
}

struct LookupRequest: Codable, Sendable {
    let type: String
    let id: String
    let surface: String
    let context: String?
    let bounds: SelectionBounds?

    enum CodingKeys: String, CodingKey {
        case type
        case id
        case surface
        case context
        case bounds
    }

    init(
        id: String,
        surface: String,
        context: String?,
        bounds: SelectionBounds?
    ) {
        type = "lookup"
        self.id = id
        self.surface = surface
        self.context = context
        self.bounds = bounds
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        type = try container.decode(String.self, forKey: .type)
        id = try container.decode(String.self, forKey: .id)
        surface = try container.decode(String.self, forKey: .surface)
        context = try container.decodeIfPresent(String.self, forKey: .context)
        bounds = try container.decodeIfPresent(SelectionBounds.self, forKey: .bounds)
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(type, forKey: .type)
        try container.encode(id, forKey: .id)
        try container.encode(surface, forKey: .surface)
        try container.encodeIfPresent(context, forKey: .context)
        try container.encodeIfPresent(bounds, forKey: .bounds)
    }
}

struct AddRequest: Encodable, Sendable {
    let type = "add"
    let id: String
    let line: GlossLine
}

struct CardFields: Codable, Equatable, Sendable {
    var simplified: String
    var traditional: String?
    var pinyin: String
    var meaning: String
    var partOfSpeech: String?
    var classifier: String?
    var example: String?
    var examplePinyin: String?
    var exampleTranslation: String?
    var tags: [String]?
    var aiGenerated: Bool?
}

struct CardVariant: Codable, Equatable, Sendable {
    let traditional: String?
    let pinyin: String
    let meaning: String
    let partOfSpeech: String?
    let classifier: String?
    let example: String?
    let examplePinyin: String?
    let exampleTranslation: String?
}

struct CardDraft: Codable, Equatable, Sendable {
    let fields: CardFields
    let needsChoice: Bool
    let readOnly: Bool
    let variants: [CardVariant]?
}

struct SaveRequest: Encodable, Sendable {
    let type = "save"
    let id: String
    let fields: CardFields
}

struct RegenerateRequest: Encodable, Sendable {
    let type = "regenerate"
    let id: String
    let fields: CardFields
    let instruction: String?
}

struct ParentMessage: Decodable, Sendable {
    let type: String
    let id: String
    let surface: String?
    let lines: [GlossLine]?
    let notice: String?
    let message: String?
    let detail: String?
    let draft: CardDraft?
    let existingTags: [String]?
}
