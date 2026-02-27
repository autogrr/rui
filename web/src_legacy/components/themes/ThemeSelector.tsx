/*
 * Copyright (c) 2025, s0up and the autobrr contributors.
 * Copyright (c) 2026, the rui contributors.
 * SPDX-License-Identifier: AGPL-1.0-or-later
 */

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { themes, type Theme } from "@/config/themes"
import { useTheme } from "@/hooks/useTheme"
import { getThemeColors, getThemeVariation } from "@/utils/theme"
import { Check, Palette } from "lucide-react"

interface ThemeCardProps {
  theme: Theme
  isSelected: boolean
  onSelect: () => void
  onVariationSelect: (themeId: string, variationId: string) => void
}

function ThemeCard({ theme, isSelected, onSelect, onVariationSelect }: ThemeCardProps) {
  const variation = getThemeVariation(theme.id)
  const colors = getThemeColors(theme)

  return (
    <Card
      className={`cursor-pointer transition-all duration-200 hover:shadow-md h-full ${
        isSelected ? "ring-2 ring-primary" : ""
      }`}
      onClick={onSelect}
    >
      <CardHeader className="pb-2 sm:pb-3">
        <div className="flex items-center justify-between">
          <div className="text-sm sm:text-base font-semibold flex items-center gap-1 sm:gap-2">
            {theme.name}
            {isSelected && (
              <Check className="h-3 w-3 sm:h-4 sm:w-4 text-primary" />
            )}
          </div>
        </div>
        {theme.description && (
          <p className="text-xs text-muted-foreground line-clamp-2">{theme.description}</p>
        )}
      </CardHeader>
      <CardContent className="pt-0 space-y-2 sm:space-y-3">
        <div className="flex items-center justify-between gap-2 flex-wrap">
          <div className="flex gap-1">
            <div
              className="w-3 h-3 sm:w-4 sm:h-4 rounded-full ring-1 ring-black/10 dark:ring-white/10"
              style={{ background: colors.primary + " !important" }}
            />
            <div
              className="w-3 h-3 sm:w-4 sm:h-4 rounded-full ring-1 ring-black/10 dark:ring-white/10"
              style={{ background: colors.secondary + " !important" }}
            />
            <div
              className="w-3 h-3 sm:w-4 sm:h-4 rounded-full ring-1 ring-black/10 dark:ring-white/10"
              style={{ background: colors.accent + " !important" }}
            />
          </div>

          {colors.variations && colors.variations.length > 0 && (
            <div className="flex gap-1">
              {colors.variations.map((v) => {
                const selected = variation === v.id
                return (
                  <button
                    key={v.id}
                    onClick={(e) => {
                      e.stopPropagation()
                      onVariationSelect(theme.id, v.id)
                    }}
                    className={`w-3 h-3 sm:w-4 sm:h-4 rounded-full transition-all ${
                      selected ? "ring-2 ring-black dark:ring-white" : "ring-1 ring-black/10 dark:ring-white/10"
                    }`}
                    style={{ background: v.color + " !important" }}
                  />
                )
              })}
            </div>
          )}
        </div>

        <div className="flex items-center gap-1 sm:gap-2">
          <Badge variant="outline" className="text-xs px-1.5 sm:px-2">
            Free
          </Badge>
        </div>
      </CardContent>
    </Card>
  )
}

export function ThemeSelector() {
  const { theme: currentTheme, setTheme, setVariation } = useTheme()

  const handleVariationSelect = (themeId: string, variationId: string) => {
    setTheme(themeId)
    setVariation(variationId)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Palette className="h-5 w-5" />
          Theme Selection
        </CardTitle>
        <CardDescription>
          Choose from available themes.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="grid grid-cols-2 md:grid-cols-3 gap-2 sm:gap-3">
          {themes.map((theme) => (
            <ThemeCard
              key={theme.id}
              theme={theme}
              isSelected={currentTheme === theme.id}
              onSelect={() => setTheme(theme.id)}
              onVariationSelect={handleVariationSelect}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
