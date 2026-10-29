// Carries Mosaic Fantasy's website colors into restrained, accessible iPhone components.
import SwiftUI

enum MosaicPalette {
    static let ground = Color(hex: 0x0b0e15)
    static let surface = Color(hex: 0x141822)
    static let raised = Color(hex: 0x1a1f2c)
    static let ink = Color(hex: 0xeceef5)
    static let soft = Color(hex: 0xa0a7ba)
    static let faint = Color(hex: 0x70778b)
    static let rule = Color(hex: 0x252b3a)
    static let brand = Color(hex: 0x8499ff)
    static let bad = Color(hex: 0xff8a8f)
    static let gold = Color(hex: 0xf0c050)
    static let goldSurface = Color(hex: 0x2e2610)

    static func sport(_ key: String) -> Color {
        switch key.lowercased() {
        case "cbb": Color(hex: 0xff9547)
        case "nba": Color(hex: 0xa48bff)
        case "wnba": Color(hex: 0xff8fb3)
        case "nhl": Color(hex: 0x4fd1e0)
        case "nfl": Color(hex: 0x62d27a)
        case "mlb": Color(hex: 0xff7676)
        default: soft
        }
    }
}

extension Color {
    init(hex: UInt32) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xff) / 255,
            green: Double((hex >> 8) & 0xff) / 255,
            blue: Double(hex & 0xff) / 255,
            opacity: 1
        )
    }
}

struct SportTag: View {
    let sport: String

    var body: some View {
        let color = MosaicPalette.sport(sport)
        HStack(spacing: 8) {
            Circle()
                .fill(color)
                .frame(width: 8, height: 8)
                .shadow(color: color.opacity(0.45), radius: 6, x: 0, y: 1)
                .accessibilityHidden(true)
            Text(sport.uppercased())
                .font(.caption.weight(.semibold))
                .foregroundStyle(MosaicPalette.ink)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(sport.uppercased())
    }
}

struct StatePanel: View {
    let symbol: String
    let title: String
    let detail: String
    var actionTitle: String? = nil
    var action: (() -> Void)? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: symbol)
                .font(.title2)
                .foregroundStyle(MosaicPalette.brand)
                .accessibilityHidden(true)
            Text(title).font(.headline).foregroundStyle(MosaicPalette.ink)
            Text(detail).font(.subheadline).foregroundStyle(MosaicPalette.soft)
            if let actionTitle, let action {
                Button(actionTitle, action: action)
                    .buttonStyle(.borderedProminent)
                    .tint(MosaicPalette.brand)
                    .foregroundStyle(MosaicPalette.ground)
                    .padding(.top, 4)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(20)
        .background(MosaicPalette.surface, in: RoundedRectangle(cornerRadius: 16))
    }
}
