import Foundation
import XCTest
@testable import call_analyzer_lookup

final class LookupTests: XCTestCase {
    func testLookupRequestRoundTripsAsJSON() throws {
        let request = LookupRequest(
            id: "1",
            surface: "行",
            context: "去银行",
            bounds: SelectionBounds(x: 1, y: 2, width: 3, height: 4)
        )

        let data = try JSONEncoder().encode(request)
        let object = try XCTUnwrap(
            JSONSerialization.jsonObject(with: data) as? [String: Any]
        )
        XCTAssertEqual(object["type"] as? String, "lookup")
        XCTAssertEqual(object["surface"] as? String, "行")
        XCTAssertEqual(object["context"] as? String, "去银行")

        let decoded = try JSONDecoder().decode(LookupRequest.self, from: data)
        XCTAssertEqual(decoded.id, request.id)
        XCTAssertEqual(decoded.surface, request.surface)
        XCTAssertEqual(decoded.context, request.context)
        XCTAssertEqual(decoded.bounds?.width, request.bounds?.width)
    }
}
