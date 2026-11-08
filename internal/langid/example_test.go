package langid_test

import (
	"fmt"

	"natural-syntax-ls/internal/langid"
)

// Detection returns a Wiktionary code per line, or "" for text too short to tell; hover then borrows a nearby line's language.
func ExampleDetect() {
	fmt.Println(langid.Languages())
	for _, text := range []string{
		"The cat sleeps on the warm windowsill.",
		"El gato duerme junto a la ventana.",
		"Le chat dort près de la fenêtre.",
		"Кошка спит на тёплом подоконнике.",
		"猫は窓のそばで寝ています。",
		"القطة نائمة بجانب النافذة.",
		"ok",
	} {
		fmt.Printf("%q\n", langid.Detect(text))
	}
	// Output:
	// [en fr es de it pt ru ja zh ar ca uk ko]
	// "en"
	// "es"
	// "fr"
	// "ru"
	// "ja"
	// "ar"
	// ""
}
