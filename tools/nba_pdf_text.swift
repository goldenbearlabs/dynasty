// Extract official injury-report text using macOS PDFKit. No third-party PDF
// dependency or access to application/account data is needed.
import Foundation
import PDFKit

for path in CommandLine.arguments.dropFirst() {
    guard let document = PDFDocument(url: URL(fileURLWithPath: path)) else {
        fputs("Cannot read PDF: \(path)\n", stderr)
        exit(1)
    }
    var text = ""
    for i in 0..<document.pageCount {
        guard let page = document.page(at: i), let body = page.string else {
            fputs("Cannot extract PDF page: \(path)\n", stderr)
            exit(1)
        }
        text += body + "\n"
    }
    try text.write(toFile: path + ".txt", atomically: true, encoding: .utf8)
}
